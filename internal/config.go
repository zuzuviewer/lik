package internal

type CertConfig struct{
	InsecureSkipVerify bool `json:"insecureSkipVerify" yaml:"insecureSkipVerify"`
	ClientCertFile string `json:"clientCertFile" yaml:"clientCertFile"`
}