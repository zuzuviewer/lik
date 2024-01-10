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
	"os"
	"strconv"
	"strings"
	"time"
)

func (r *Request) Do(likConfig *LikConfig, out io.Writer) {
	req := r.Clone()
	req.prepare(likConfig)
	req.do(out)
}

func (r *Request) do(out io.Writer) {
	var (
		err      error
		cancel   context.CancelFunc
		request  *http.Request
		response *http.Response
	)
	request, cancel, err = r.packageRequest()
	if err != nil {
		log.Printf("package request %s %s failed, %v", r.Namespace, r.Name, err)
		if r.ExitOnFailure {
			os.Exit(1)
		}
		return
	}
	if cancel != nil {
		defer cancel()
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
	r.printResponse(response, duration, out)
	if r.ExitOnFailure && response.StatusCode >= http.StatusBadRequest {
		os.Exit(1)
	}
}

func (r *Request) packageRequest() (*http.Request, context.CancelFunc, error) {
	var (
		err     error
		ctx     context.Context
		body    io.Reader
		timeout time.Duration
		request *http.Request
		cancel  context.CancelFunc
	)
	body, err = r.parseBody()
	if err != nil {
		return nil, nil, err
	}
	request, err = http.NewRequest(r.Method, r.Url, body)
	if err != nil {
		return nil, nil, err
	}
	request.Header = r.Headers
	// request.URL.RawQuery
	if len(r.Queries) > 0 {
		request.URL.RawQuery = r.Queries.Encode()
	}
	timeout, err = r.parseTimeout()
	if err != nil {
		return nil, nil, err
	}
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		request = request.WithContext(ctx)
	}
	return request, cancel, nil
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
		if r.Headers == nil {
			r.Headers = make(http.Header, 1)
		}
		r.Headers.Set("Content-Type", "application/json")
		return bytes.NewBuffer(b), nil
	case FormData:
		formData = make([]FormDataBody, 0)
		err = json.Unmarshal(b, &formData)
		if err != nil {
			return nil, err
		}
		buffer := bytes.NewBuffer(make([]byte, 0))
		writer := multipart.NewWriter(buffer)
		if r.Headers == nil {
			r.Headers = make(http.Header, 1)
		}
		r.Headers.Set("Content-Type", writer.FormDataContentType())
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
	case Form:
		if r.Headers == nil {
			r.Headers = make(http.Header, 1)
		}
		r.Headers.Set("Content-Type", "application/x-www-form-urlencoded")
		return bytes.NewBuffer(b), nil
	case Raw:
		return bytes.NewBuffer(b), nil
	default:
		return nil, fmt.Errorf("unsupported body type " + string(r.Body.Type))
	}
}

func (r *Request) printResponse(response *http.Response, duration time.Duration, out io.Writer) {
	builder := new(strings.Builder)
	builder.WriteString("---------------")
	builder.WriteString(r.Namespace + " " + r.Name + "---------------\n")
	if r.Response.ShowUrl != nil && *r.Response.ShowUrl {
		builder.WriteString(r.Url + "\n")
	}
	if r.Response.ShowCode == nil || *r.Response.ShowCode {
		builder.WriteString(strconv.Itoa(response.StatusCode) + "\n")
	}
	if r.Response.ShowTimeConsumption != nil && *r.Response.ShowTimeConsumption {
		builder.WriteString(duration.String() + "\n")
	}
	// todo more beautiful format
	if r.Response.ShowHeader != nil && *r.Response.ShowHeader {
		header, _ := json.Marshal(response.Header)
		builder.WriteString(string(header) + "\n")
	}
	// todo more beautiful format
	if r.Response.ShowBody != nil && *r.Response.ShowBody {
		body, _ := io.ReadAll(response.Body)
		builder.WriteString(string(body) + "\n")
	}
	fmt.Fprint(out, builder.String())
}

func (r *Request) parseTimeout() (time.Duration, error) {
	timeout := r.Timeout
	if timeout == "" {
		return 0, nil
	}
	return time.ParseDuration(timeout)
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
