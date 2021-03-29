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
	"net/url"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"

	"sigs.k8s.io/cluster-api-provider-vsphere/api/v1alpha3"
)

var sessionCache = map[string]Session{}
var sessionMU sync.Mutex

// Session is a vSphere session with a configured Finder.
type Session struct {
	*govmomi.Client
	Finder     *find.Finder
	datacenter *object.Datacenter
}

type GetOrCreateContext struct {
	context context.Context
	logger  logr.Logger
}

func NewGetOrCreateContext(ctx context.Context, logger logr.Logger) GetOrCreateContext {
	return GetOrCreateContext{
		context: ctx,
		logger:  logger.WithName("session"),
	}
}

type Feature struct {
	EnableKeepAlive   bool
	KeepAliveDuration time.Duration
}

func DefaultFeature() Feature {
	return Feature{
		EnableKeepAlive:   false,
		KeepAliveDuration: 0,
	}
}

// GetOrCreate gets a cached session or creates a new one if one does not
// already exist.
func GetOrCreate(
	ctx GetOrCreateContext,
	server, datacenter, username, password string, thumbprint string, feature Feature) (*Session, error) {

	sessionMU.Lock()
	defer sessionMU.Unlock()

	sessionKey := server + username + datacenter
	if session, ok := sessionCache[sessionKey]; ok {
		// if keepalive is enabled we depend upon roundtripper to reestablish the connection
		// and remove the key if it could not
		if feature.EnableKeepAlive {
			return &session, nil
		}
		if ok, _ := session.SessionManager.SessionIsActive(ctx.context); ok {
			return &session, nil
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
	client, err := newClient(ctx, sessionKey, soapURL, thumbprint, feature)
	if err != nil {
		return nil, err
	}

	session := Session{Client: client}
	session.UserAgent = v1alpha3.GroupVersion.String()

	// Assign the finder to the session.
	session.Finder = find.NewFinder(session.Client.Client, false)

	// Assign the datacenter if one was specified.
	dc, err := session.Finder.DatacenterOrDefault(ctx.context, datacenter)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to find datacenter %q", datacenter)
	}
	session.datacenter = dc
	session.Finder.SetDatacenter(dc)

	// Cache the session.
	sessionCache[sessionKey] = session

	// TODO(akutz) Reintroduce the logger.
	//ctx.Logger.V(2).Info("cached vSphere client session", "server", server, "datacenter", datacenter)

	return &session, nil
}

func newClient(ctx GetOrCreateContext, sessionKey string, url *url.URL, thumprint string, feature Feature) (*govmomi.Client, error) {
	insecure := thumprint == ""
	soapClient := soap.NewClient(url, insecure)
	if !insecure {
		soapClient.SetThumbprint(url.Host, thumprint)
	}

	vimClient, err := vim25.NewClient(ctx.context, soapClient)
	if err != nil {
		return nil, err
	}

	c := &govmomi.Client{
		Client:         vimClient,
		SessionManager: session.NewManager(vimClient),
	}

	if feature.EnableKeepAlive {
		vimClient.RoundTripper = session.KeepAliveHandler(vimClient.RoundTripper, feature.KeepAliveDuration, func(tripper soap.RoundTripper) error {
			// we tried implementing
			// c.Login here but the client once logged out
			// keeps errong in invalid username or password
			// we tried with cached username and password in session still the error persisted
			// hence we just clear the cache and expect the client to
			// be recreated in next GetOrCreate call
			_, err := methods.GetCurrentTime(ctx.context, tripper)
			if err != nil {
				ctx.logger.Error(err, "failed to keep alive govmomi client")
				ClearCache(sessionKey)
			}
			return err
		})
	}

	if err := c.Login(ctx.context, url.User); err != nil {
		return nil, err
	}

	return c, nil
}

func ClearCache(sessionKey string) {
	sessionMU.Lock()
	defer sessionMU.Unlock()
	delete(sessionCache, sessionKey)
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
