package internal

import "strings"

type LikConfig struct {
	Env      []EnvConfig    `json:"env" yaml:"env"`
	Request  RequestConfig  `json:"request" yaml:"request"`
	Response ResponseConfig `json:"response" yaml:"response"`
}

type EnvConfig struct {
	Namespace string            `json:"namespace" yaml:"namespace"`
	Env       map[string]string `json:"env" yaml:"env"`
}

type RequestConfig struct {
	Timeout string `json:"timeout" yaml:"timeout"`
}

type ResponseConfig struct {
	response `json:",inline" yaml:",inline"`
}

type CertConfig struct {
	InsecureSkipVerify bool   `json:"insecureSkipVerify" yaml:"insecureSkipVerify"`
	ClientCertFile     string `json:"clientCertFile" yaml:"clientCertFile"`
}

func (c *LikConfig) replaceMacro(namespace, data string) string {
	if data == "" || !strings.Contains(data, "${") || !strings.Contains(data, "}") || len(c.Env) == 0 {
		return data
	}
	var (
		globalEnv    map[string]string
		namespaceEnv map[string]string
	)
	for i, v := range c.Env {
		if v.Namespace == "" {
			globalEnv = c.Env[i].Env
		} else if v.Namespace == namespace {
			namespaceEnv = c.Env[i].Env
		}
		if globalEnv != nil && namespaceEnv != nil {
			break
		}
	}
	var (
		stringbuiler = new(strings.Builder)
		dataLen      = len(data)
		startIndex   = -1
	)
	stringbuiler.Grow(dataLen)
	for i := 0; i < dataLen; {
		if data[i] == '$' && i+3 < dataLen && data[i+1] == '{' {
			i += 2
			startIndex = i
			continue
		}
		if data[i] == '}' && startIndex != -1 && startIndex+2 < i {
			macro := data[startIndex:i]
			stringbuiler.WriteString(findMacroValue(macro, globalEnv, namespaceEnv))
			startIndex = -1
			i++
			continue
		}
		if startIndex == -1 {
			stringbuiler.WriteByte(data[i])
		}
		i++
	}
	return stringbuiler.String()
}

func findMacroValue(macro string, globalEnv, namespaceEnv map[string]string) string {
	if len(namespaceEnv) > 0 {
		v, ok := namespaceEnv[macro]
		if ok {
			return v
		}
	}
	if len(globalEnv) > 0 {
		v, ok := globalEnv[macro]
		if ok {
			return v
		}
	}
	return "${" + macro + "}"
}
