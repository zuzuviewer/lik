package internal

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type bodyType string

const (
	Json     bodyType = "json"
	FormData bodyType = "form-data"
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
	ShowUrl             bool `json:"showUrl" yaml:"showUrl"`
	ShowHeader          bool `json:"showHeader" yaml:"showHeader"` // show response header,default false
	ShowCode            bool `json:"showCode" yaml:"showCode"`     //  show response code, default true
	ShowBody            bool `json:"showBody" yaml:"showBody"`     // show response body, default true
	ShowTimeConsumption bool `json:"showTimeConsumption" yaml:"showTimeConsumption"`
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

func (r *Request) Do() {
	var (
		err      error
		body     io.Reader
		timeout  time.Duration
		request  *http.Request
		response *http.Response
	)
	body, err = r.parseBody()
	if err != nil {
		log.Printf("request %s %s parse body failed, %v", r.Namespace, r.Name, err)
		if r.ExitOnFailure {
			os.Exit(1)
		}
		return
	}
	request, err = http.NewRequest(r.Method, r.Url, body)
	if err != nil {
		log.Printf("request %s %s create failed, %v", r.Namespace, r.Name, err)
		if r.ExitOnFailure {
			os.Exit(1)
		}
		return
	}
	timeout, err = r.parseTimeout()
	if err != nil {
		log.Printf("request %s %s parse timeout failed, %v", r.Namespace, r.Name, err)
		if r.ExitOnFailure {
			os.Exit(1)
		}
		return
	}
	if timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		request = request.WithContext(ctx)
	}
	start := time.Now()
	response, err = r.client(request).Do(request)
	if err != nil {
		log.Printf("request %s %s failed, %v", r.Namespace, r.Name, err)
		if r.ExitOnFailure {
			os.Exit(1)
		}
		return
	}
	duration := time.Since(start)
	defer response.Body.Close()
	r.printResponse(response, duration)
	if r.ExitOnFailure && response.StatusCode >= http.StatusBadRequest {
		os.Exit(1)
	}
}

func (r *Request) parseBody() (io.Reader, error) {
	var (
		err        error
		b          []byte
		formData   []FormDataBody
		fileWriter io.Writer
		file       *os.File
	)
	if r.Body.Data == nil {
		return nil, nil
	}
	b, err = json.Marshal(r.Body.Data)
	if err != nil {
		return nil, err
	}
	switch r.Body.Type {
	case "", Json:
		return bytes.NewBuffer(b), nil
	case FormData:
		formData = make([]FormDataBody, 0)
		err = json.Unmarshal(b, &formData)
		if err != nil {
			return nil, err
		}
		buffer := bytes.NewBuffer(make([]byte, 0))
		writer := multipart.NewWriter(buffer)
		for _, data := range formData {
			switch data.Type {
			case "kv":
				writer.WriteField(data.Name, data.Value)
			case "file":
				fileWriter, err = writer.CreateFormFile(data.Name, data.Filename)
				if err != nil {
					return nil, err
				}
				if data.FilePath == "" && data.Content == "" {
					return nil, fmt.Errorf("invalid form data file, file path or file content required")
				}
				if data.FilePath == "" {
					_, err = fileWriter.Write([]byte(data.Content))
					if err != nil {
						return nil, err
					}
				} else {
					file, err = os.Open(data.FilePath)
					if err != nil {
						_, err = fileWriter.Write([]byte(data.Content))
						if err != nil {
							return nil, err
						}
					} else {
						b, err = io.ReadAll(file)
						if err != nil {
							_, err = fileWriter.Write([]byte(data.Content))
							if err != nil {
								return nil, err
							}
						} else {
							_, err = fileWriter.Write(b)
							if err != nil {
								_, err = fileWriter.Write([]byte(data.Content))
								if err != nil {
									return nil, err
								}
							}
						}
					}
				}
			}
		}
		return buffer, nil
	default:
		return nil, fmt.Errorf("unsupported body type " + string(r.Body.Type))
	}
}

func (r *Request) printResponse(response *http.Response, duration time.Duration) {
	builder := new(strings.Builder)
	builder.WriteString("\n")
	builder.WriteString(r.Namespace + " " + r.Name + "\n")
	if r.Response.ShowUrl {
		builder.WriteString(r.Url + "\n")
	}
	if r.Response.ShowCode {
		builder.WriteString(strconv.Itoa(response.StatusCode) + "\n")
	}
	if r.Response.ShowTimeConsumption {
		builder.WriteString(duration.String() + "\n")
	}
	// todo more beautiful format
	if r.Response.ShowHeader {
		header, _ := json.Marshal(response.Header)
		builder.WriteString(string(header) + "\n")
	}
	// todo more beautiful format
	if r.Response.ShowBody {
		body, _ := io.ReadAll(response.Body)
		builder.WriteString(string(body) + "\n")
	}
	fmt.Fprint(os.Stdout, builder.String())
}

func (r *Request) parseTimeout() (time.Duration, error) {
	return time.ParseDuration(r.Timeout)
}

func (r *Request) client(request *http.Request) *http.Client {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if request.URL.Scheme == "https" {
		host, _, err := net.SplitHostPort(request.Host)
		if err != nil {
			host = request.Host
		}

		tr.TLSClientConfig = &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: r.CertConfig.InsecureSkipVerify,
			Certificates:       readClientCert(r.CertConfig.ClientCertFile),
			MinVersion:         tls.VersionTLS12,
		}
	}
	return &http.Client{
		Transport: tr,
	}
}

// readClientCert read pem client certificate file
func readClientCert(filename string) []tls.Certificate {
	if filename == "" {
		return nil
	}
	var (
		pkeyPem []byte
		certPem []byte
	)

	// read client certificate file (must include client private key and certificate)
	certFileBytes, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("failed to read client certificate file: %v", err)
	}

	for {
		block, rest := pem.Decode(certFileBytes)
		if block == nil {
			break
		}
		certFileBytes = rest

		if strings.HasSuffix(block.Type, "PRIVATE KEY") {
			pkeyPem = pem.EncodeToMemory(block)
		}
		if strings.HasSuffix(block.Type, "CERTIFICATE") {
			certPem = pem.EncodeToMemory(block)
		}
	}

	cert, err := tls.X509KeyPair(certPem, pkeyPem)
	if err != nil {
		log.Fatalf("unable to load client cert and key pair: %v", err)
	}
	return []tls.Certificate{cert}
}
