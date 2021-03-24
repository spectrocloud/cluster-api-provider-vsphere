package bootstrap

type VMBootstrapData struct {
	value []byte
	format Format
}

type Format string

const (
	// CloudConfig make the bootstrap data to be of cloud-config format
	CloudConfig Format = "cloud-config"

	// Ignition make the bootstrap data to be of Ignition format.
	Ignition Format = "ignition"
)

func (vbd *VMBootstrapData) GetValue() []byte {
	return vbd.value
}

func (vbd VMBootstrapData) SetValue(value []byte) {
	vbd.value = value
}

func (vbd *VMBootstrapData) SetFormat(format Format) {
	vbd.format = format
}

func (vbd *VMBootstrapData) GetFormat() Format {
	return vbd.format
}
