package dockerproxy

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/golang/glog"
)

const (
	HeaderConnection = "Connection"
	HeaderUpgrade    = "Upgrade"
)

type RequestModifierFunc func(req *http.Request) (*http.Request, error)

type dockerProxy struct {
	dockerHost      string
	requestModifier RequestModifierFunc
	internalProxy   *httputil.ReverseProxy
}

type connCloser interface {
	CloseRead() error
	CloseWrite() error
}

var fakeDockerURL = mustParse("http://dockerhost")

func mustParse(str string) *url.URL {
	u, err := url.Parse(str)
	if err != nil {
		panic(err)
	}
	return u
}

func New(requestModifierFn RequestModifierFunc) http.Handler {
	internalProxy := httputil.NewSingleHostReverseProxy(fakeDockerURL)
	internalProxy.Transport = &http.Transport{
		Dial: dialDockerWrapper,
	}
	return &dockerProxy{
		requestModifier: requestModifierFn,
		internalProxy:   internalProxy,
	}
}

// ServeHTTP handles the proxy request
func (p *dockerProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	glog.Infof("Serving %s %s\n", req.Method, req.URL.String())
	upgraded, err := p.tryUpgrade(w, req)
	if err != nil {
		p.writeError(w, err)
	}
	if upgraded {
		return
	}
	if p.requestModifier != nil {
		req, err = p.requestModifier(req)
		if err != nil {
			glog.Infof("Error modifying request: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
	p.internalProxy.ServeHTTP(w, req)
}

func (p *dockerProxy) writeError(w http.ResponseWriter, err error) {
	msg := "internal error"
	if err != nil {
		msg = err.Error()
	}
	http.Error(w, msg, http.StatusInternalServerError)
}

// IsUpgradeRequest returns true if the given request is a connection upgrade request
func isUpgradeRequest(req *http.Request) bool {
	for _, h := range req.Header[HeaderConnection] {
		if strings.Contains(strings.ToLower(h), strings.ToLower(HeaderUpgrade)) {
			return true
		}
	}
	return false
}

func (p *dockerProxy) dockerURL(req *http.Request) string {
	u := *req.URL
	u.Host = fakeDockerURL.Host
	return u.String()
}

func dialDockerWrapper(string, string) (net.Conn, error) {
	return dialDocker()
}

func dialDocker() (net.Conn, error) {
	return net.Dial("unix", "/var/run/docker.sock")
}

func (p *dockerProxy) tryUpgrade(w http.ResponseWriter, req *http.Request) (bool, error) {
	if !isUpgradeRequest(req) {
		return false, nil
	}
	backendConn, err := dialDocker()
	if err != nil {
		return true, err
	}
	backendCloser, ok := backendConn.(connCloser)
	if !ok {
		return true, fmt.Errorf("backend connection is not connection closer")
	}

	requestHijackedConn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return true, err
	}
	requestCloser, ok := requestHijackedConn.(connCloser)
	if !ok {
		return true, fmt.Errorf("request connection is not connection closer")
	}

	newRequest, err := http.NewRequest(req.Method, p.dockerURL(req), req.Body)
	if err != nil {
		return true, err
	}
	newRequest.Header = req.Header

	reqBytes := &bytes.Buffer{}
	newRequest.Write(reqBytes)

	if err = newRequest.Write(backendConn); err != nil {
		return true, err
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		_, err := io.Copy(backendConn, requestHijackedConn)
		if err != nil {
			glog.Errorf("Error proxying data from client to backend: %v", err)
		}
		wg.Done()
		backendCloser.CloseWrite()
		requestCloser.CloseRead()
	}()

	go func() {
		_, err := io.Copy(requestHijackedConn, backendConn)
		if err != nil {
			glog.Errorf("Error proxying data from backend to client: %v", err)
		}
		wg.Done()
		requestCloser.CloseWrite()
		backendCloser.CloseRead()
	}()

	wg.Wait()
	return true, nil
}
