/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package session

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/session/keepalive"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"

	"sigs.k8s.io/cluster-api-provider-vsphere/api/v1alpha3"
	ctrl "sigs.k8s.io/controller-runtime"
)

// TODO make these configurable
const EnableKeepAliveSession = true
const KeepAliveIntervalInMinute = 5

// TODO pass logger down from controller
var govlog = ctrl.Log.WithName("govmomi client")

var sessionCache = map[string]Session{}
var sessionMU sync.Mutex

// Session is a vSphere session with a configured Finder.
type Session struct {
	*govmomi.Client
	Finder     *find.Finder
	datacenter *object.Datacenter
}

// GetOrCreate gets a cached session or creates a new one if one does not
// already exist.
func GetOrCreate(
	ctx context.Context,
	server, datacenter, username, password string) (*Session, error) {

	if EnableKeepAliveSession {
		return GetOrCreateKeepAliveSession(ctx, server, datacenter, username, password)
	}

	return GetOrCreateNonKeepAliveSession(ctx, server, datacenter, username, password)
}

// GetOrCreate gets a cached session or creates a new one if one does not
// already exist.
// If session exists, this call will still go to vcenter to validate if session
// is active, thus making at least one api call to vcenter everytime this is called
// while GetOrCreateKeepAliveSession will only do on call per configured interval
func GetOrCreateNonKeepAliveSession(
	ctx context.Context,
	server, datacenter, username, password string) (*Session, error) {

	sessionMU.Lock()
	defer sessionMU.Unlock()

	sessionKey := server + username + datacenter
	if currentSession, ok := sessionCache[sessionKey]; ok {
		if ok, _ := currentSession.SessionManager.SessionIsActive(ctx); ok {
			return &currentSession, nil
		}
	}

	soapURL, err := soap.ParseURL(server)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing vSphere URL %q", server)
	}
	if soapURL == nil {
		return nil, errors.Errorf("error parsing vSphere URL %q", server)
	}

	soapURL.User = url.UserPassword(username, password)

	// Temporarily setting the insecure flag True
	// TODO(ssurana): handle the certs better
	client, err := govmomi.NewClient(ctx, soapURL, true)
	if err != nil {
		return nil, errors.Wrapf(err, "error setting up new vSphere SOAP client")
	}

	currentSession := Session{Client: client}
	currentSession.UserAgent = v1alpha3.GroupVersion.String()

	// Assign the finder to the session.
	currentSession.Finder = find.NewFinder(currentSession.Client.Client, false)

	// Assign the datacenter if one was specified.
	dc, err := currentSession.Finder.DatacenterOrDefault(ctx, datacenter)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to find datacenter %q", datacenter)
	}
	currentSession.datacenter = dc
	currentSession.Finder.SetDatacenter(dc)

	// Cache the session.
	sessionCache[sessionKey] = currentSession

	// TODO(akutz) Reintroduce the logger.
	//ctx.Logger.V(2).Info("cached vSphere client session", "server", server, "datacenter", datacenter)

	return &currentSession, nil
}

// GetOrCreateKeepAliveSession gets a cached keepalive session or creates a new one if one does not exist
// this session will not do a active check for every call, instead it'll do a keepalive check every configured
// interval to keep the session alive
func GetOrCreateKeepAliveSession(
	ctx context.Context,
	server, datacenter, username, password string) (*Session, error) {

	sessionMU.Lock()
	defer sessionMU.Unlock()

	sessionKey := server + username + datacenter
	currentSession, ok := sessionCache[sessionKey]

	if ok {
		return &currentSession, nil
	}

	// govmomi client
	client, err := createGovmomiClientWithKeepAlive(ctx, sessionKey, server, username, password)
	if err != nil {
		return nil, err
	}

	currentSession = Session{Client: client}
	currentSession.UserAgent = v1alpha3.GroupVersion.String()
	// Assign the finder to the currentSession.
	currentSession.Finder = find.NewFinder(currentSession.Client.Client, false)

	// Assign the datacenter if one was specified.
	dc, err := currentSession.Finder.DatacenterOrDefault(ctx, datacenter)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to find datacenter %q", datacenter)
	}

	currentSession.datacenter = dc
	currentSession.Finder.SetDatacenter(dc)

	// Cache the currentcurrentSession.
	sessionCache[sessionKey] = currentSession
	return &currentSession, nil
}

func createGovmomiClientWithKeepAlive(ctx context.Context, sessionKey, server, username, password string) (*govmomi.Client, error) {
	//get vcenter URL
	vCenterURL, err := getVCenterURL(server, username, password)
	if err != nil {
		return nil, err
	}

	insecure := true

	soapClient := soap.NewClient(vCenterURL, insecure)
	vimClient, err := vim25.NewClient(ctx, soapClient)
	if err != nil {
		return nil, err
	}

	c := &govmomi.Client{
		Client:         vimClient,
		SessionManager: session.NewManager(vimClient),
	}

	send := func() error {
		ctx := context.Background()
		_, err := methods.GetCurrentTime(ctx, vimClient.RoundTripper)
		if err != nil {
			govlog.Error(err, "failed to keep alive govmomi client")
			ClearCache(sessionKey)
		}
		return err
	}

	// this starts the keep alive handler when Login is called, and stops the handler when Logout is called
	// it'll also stop the handler when send() returns error, so we wrap around the default send()
	// with err check to clear cache in case of error
	vimClient.RoundTripper = keepalive.NewHandlerSOAP(vimClient.RoundTripper, KeepAliveIntervalInMinute*time.Minute, send)

	// Only login if the URL contains user information.
	if vCenterURL.User != nil {
		govlog.V(0).Info("########### login to vcenter for soap client ###############")
		err = c.Login(ctx, vCenterURL.User)
		if err != nil {
			return nil, err
		}
	}

	return c, nil
}

func ClearCache(sessionKey string) {
	sessionMU.Lock()
	defer sessionMU.Unlock()
	delete(sessionCache, sessionKey)
}

func getVCenterURL(vCenterServer string, vCenterUsername string, vCenterPassword string) (*url.URL, error) {
	//create vcenter URL
	vCenterURL, err := url.Parse(fmt.Sprintf("https://%s/sdk", vCenterServer))
	if err != nil {
		return nil, errors.Errorf("invalid vCenter server")

	}
	vCenterURL.User = url.UserPassword(vCenterUsername, vCenterPassword)

	return vCenterURL, nil
}

// FindByBIOSUUID finds an object by its BIOS UUID.
//
// To avoid comments about this function's name, please see the Golang
// WIKI https://github.com/golang/go/wiki/CodeReviewComments#initialisms.
// This function is named in accordance with the example "XMLHTTP".
func (s *Session) FindByBIOSUUID(ctx context.Context, uuid string) (object.Reference, error) {
	return s.findByUUID(ctx, uuid, false)
}

// FindByInstanceUUID finds an object by its instance UUID.
func (s *Session) FindByInstanceUUID(ctx context.Context, uuid string) (object.Reference, error) {
	return s.findByUUID(ctx, uuid, true)
}

func (s *Session) findByUUID(ctx context.Context, uuid string, findByInstanceUUID bool) (object.Reference, error) {
	if s.Client == nil {
		return nil, errors.New("vSphere client is not initialized")
	}
	si := object.NewSearchIndex(s.Client.Client)
	ref, err := si.FindByUuid(ctx, s.datacenter, uuid, true, &findByInstanceUUID)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding object by uuid %q", uuid)
	}
	return ref, nil
}
