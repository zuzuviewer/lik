package internal

import (
	"net/http"
	"net/url"
)

type bodyType string

const (
	Json     bodyType = "json"
	FormData bodyType = "form-data"
	Form     bodyType = "form"
	Raw      bodyType = "raw"
)

type Request struct {
	Namespace     string      `json:"namespace" yaml:"namespace"`
	Name          string      `json:"name" yaml:"name"`
	Method        string      `json:"method" yaml:"method"`
	Url           string      `json:"url" yaml:"url"`
	Headers       http.Header `json:"headers" yaml:"headers"`
	Queries       url.Values  `json:"queries" yaml:"queries"`
	Body          Body        `json:"body" yaml:"body"`
	Timeout       string      `json:"timeout" yaml:"timeout"`
	Skip          bool        `json:"skip" yaml:"skip"`
	ExitOnFailure bool        `json:"exitOnFailure" yaml:"exitOnFailure"`
	Response      response    `json:"response" yaml:"response"`
	CertConfig    `json:"inline" yaml:"inline"`
}

type Body struct {
	Type bodyType    `json:"type"`
	Data interface{} `json:"data"`
}

type FormDataBody struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	Filename string `json:"filename"`
	FilePath string `json:"filePath"`
	Content  string `json:"content"`
}

type response struct {
	ShowUrl             *bool `json:"showUrl" yaml:"showUrl"`                         // show request url, default false
	ShowHeader          *bool `json:"showHeader" yaml:"showHeader"`                   // show response header,default false
	ShowCode            *bool `json:"showCode" yaml:"showCode"`                       //  show response code, default true
	ShowBody            *bool `json:"showBody" yaml:"showBody"`                       // show response body, default false
	ShowTimeConsumption *bool `json:"showTimeConsumption" yaml:"showTimeConsumption"` // show request time consumption, default false
}

func (r *Request) ShouldRequest(ns, name string) bool {
	if r.Skip {
		return false
	}
	if ns == "" && name == "" {
		return true
	}
	if ns == "" {
		return name == r.Name
	}
	if name == "" {
		return ns == r.Namespace
	}
	return ns == r.Namespace && name == r.Name
}

func (r *Request) Clone() *Request {
	ret := new(Request)
	*ret = *r
	ret.Headers = r.Headers.Clone()
	ret.Queries = url.Values(http.Header(r.Queries).Clone())
	return ret
}

func (r *Request) prepare(likConfig *LikConfig) {
	// use namespace config if not set
	namespaceConfig, exist := likConfig.getNamespaceConfig(r.Namespace)
	if exist {
		if r.Timeout == "" {
			r.Timeout = namespaceConfig.Request.Timeout
		}
		if r.Response.ShowCode == nil {
			r.Response.ShowCode = namespaceConfig.Response.ShowCode
		}
		if r.Response.ShowBody == nil {
			r.Response.ShowBody = namespaceConfig.Response.ShowBody
		}
		if r.Response.ShowHeader == nil {
			r.Response.ShowHeader = namespaceConfig.Response.ShowHeader
		}
		if r.Response.ShowUrl == nil {
			r.Response.ShowUrl = namespaceConfig.Response.ShowUrl
		}
		if r.Response.ShowTimeConsumption == nil {
			r.Response.ShowTimeConsumption = namespaceConfig.Response.ShowTimeConsumption
		}
	}
	// use global config if not set
	if r.Timeout == "" {
		r.Timeout = likConfig.Request.Timeout
	}
	if r.Response.ShowCode == nil {
		r.Response.ShowCode = likConfig.Response.ShowCode
	}
	if r.Response.ShowBody == nil {
		r.Response.ShowBody = likConfig.Response.ShowBody
	}
	if r.Response.ShowHeader == nil {
		r.Response.ShowHeader = likConfig.Response.ShowHeader
	}
	if r.Response.ShowUrl == nil {
		r.Response.ShowUrl = likConfig.Response.ShowUrl
	}
	if r.Response.ShowTimeConsumption == nil {
		r.Response.ShowTimeConsumption = likConfig.Response.ShowTimeConsumption
	}
	// replace macro
	r.Url = likConfig.replaceMacro(r.Namespace, r.Url)
	r.Timeout = likConfig.replaceMacro(r.Namespace, r.Timeout)
}
