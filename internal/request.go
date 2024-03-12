package internal

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"html"
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

func (r *Request) Do(likConfig *LikConfig, out io.Writer) error {
	req := r.Clone()
	req.prepare(likConfig)
	if r.Repeat <= 1 {
		return req.do(out)
	}
	for i := 0; i < r.Repeat; i++ {
		if err := req.do(out); err != nil {
			return err
		}
	}
	return nil
}

func (r *Request) do(out io.Writer) error {
	var (
		err     error
		cancel  context.CancelFunc
		request *http.Request
		resp    *http.Response
	)
	request, cancel, err = r.packageRequest()
	if err != nil {
		log.Printf("package request %s %s failed, %v", r.Namespace, r.Name, err)
		return err
	}
	if cancel != nil {
		defer cancel()
	}
	start := time.Now()
	resp, err = r.getClient(request).Do(request)
	if err != nil {
		log.Printf("request %s %s failed, %v", r.Namespace, r.Name, err)
		return err
	}
	duration := time.Since(start)
	defer resp.Body.Close()
	r.printResponse(resp, duration, out)
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("request %s %s failed, code %d", r.Namespace, r.Name, resp.StatusCode)
	}
	return nil
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
	if r.Username != "" && r.Password != "" {
		request.SetBasicAuth(r.Username, r.Password)
	}
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

	if r.Response.ShowHeader != nil && *r.Response.ShowHeader {
		builder.WriteString("Headers:\n")
		for k, v := range response.Header {
			builder.WriteString("  ")
			builder.WriteString(k)
			builder.WriteString(": ")
			builder.WriteString(fmt.Sprintf("%v", v))
			builder.WriteString("\n")
		}
	}

	if r.Response.ShowBody != nil && *r.Response.ShowBody {
		builder.WriteString(formatBody(response.Header, response.Body) + "\n")
	}
	fmt.Fprint(out, builder.String())
}

func formatBody(headers http.Header, body io.Reader) string {
	b, _ := io.ReadAll(body)
	contentType := headers.Get("Content-Type")
	if contentType == "application/json" {
		dst := bytes.NewBuffer(make([]byte, 0))
		err := json.Indent(dst, b, "", "    ")
		if err != nil {
			return string(b)
		} else {
			return dst.String()
		}
	}
	if contentType == "text/html" {
		return html.UnescapeString(string(b))
	}
	return string(b)
}

func (r *Request) parseTimeout() (time.Duration, error) {
	timeout := r.Timeout
	if timeout == "" {
		return 0, nil
	}
	return time.ParseDuration(timeout)
}

func (r *Request) getClient(request *http.Request) *http.Client {
	if r.client != nil {
		return r.client
	}
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
	r.client = &http.Client{
		Transport: tr,
	}
	return r.client
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
