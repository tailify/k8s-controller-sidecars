// The majority of this file was borrowed from https://github.com/rmohr/kubernetes-custom-exec

package lib

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/rest"
)

type RoundTripCallback func(conn *websocket.Conn, resp *http.Response, err error) error

type WebsocketRoundTripper struct {
	Dialer *websocket.Dialer
	Do     RoundTripCallback
}

func (d *WebsocketRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	conn, resp, err := d.Dialer.Dial(r.URL.String(), r.Header)
	if err == nil {
		defer conn.Close()
	}
	return resp, d.Do(conn, resp, err)
}

func WebsocketCallback(ws *websocket.Conn, resp *http.Response, err error) error {

	if err != nil {
		if resp != nil && resp.StatusCode != http.StatusOK {
			buf := new(bytes.Buffer)
			buf.ReadFrom(resp.Body)
			return fmt.Errorf("can't connect to console (%d): %s", resp.StatusCode, buf.String())
		}
		return fmt.Errorf("can't connect to console: %s", err.Error())
	}

	txt := ""
	for {
		_, body, err := ws.ReadMessage()
		if err != nil {
			fmt.Println(txt)
			if err == io.EOF {
				return nil
			}
			if websocket.IsCloseError(err, 1000) {
				return nil
			}
			return err
		}
		txt = txt + string(body)
	}
}

func RoundTripperFromConfig(config *rest.Config) (http.RoundTripper, error) {

	// Configure TLS
	tlsConfig, err := rest.TLSConfigFor(config)
	if err != nil {
		return nil, err
	}

	// Configure the websocket dialer
	dialer := &websocket.Dialer{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: tlsConfig,
	}

	// Create a roundtripper which will pass in the final underlying websocket connection to a callback
	rt := &WebsocketRoundTripper{
		Do:     WebsocketCallback,
		Dialer: dialer,
	}

	// Make sure we inherit all relevant security headers
	return rest.HTTPWrappersForConfig(config, rt)
}

func RequestFromConfig(config *rest.Config, pod string, container string, namespace string, cmd string) (*http.Request, error) {

	u, err := url.Parse(config.Host)
	if err != nil {
		return nil, err
	}

	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return nil, fmt.Errorf("malformed URL %s", u.String())
	}

	u.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/exec", namespace, pod)
	if container != "" {
		u.RawQuery = "command=" + cmd +
			"&container=" + container +
			"&stderr=true&stdout=true"
	}
	req := &http.Request{
		Method: http.MethodGet,
		URL:    u,
		Header: map[string][]string{},
	}

	return req, nil
}
