package dockerproxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
)

const (
	HeaderConnection = "Connection"
	HeaderUpgrade    = "Upgrade"
)

type dockerProxy struct {
	dockerHost    string
	internalProxy *httputil.ReverseProxy
}

var fakeDockerURL = mustParse("http://dockerhost")

func mustParse(str string) *url.URL {
	u, err := url.Parse(str)
	if err != nil {
		panic(err)
	}
	return u
}

func New(director func(*http.Request)) http.Handler {
	internalProxy := httputil.NewSingleHostReverseProxy(fakeDockerURL)
	internalProxy.Transport = &http.Transport{
		Dial: dialDockerWrapper,
	}
	internalProxy.Director = director
	return &dockerProxy{
		internalProxy: internalProxy,
	}
}

// ServeHTTP handles the proxy request
func (p *dockerProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	p.logMessage("Serving %s %s", req.Method, req.URL.String())
	upgraded, err := p.tryUpgrade(w, req)
	if err != nil {
		p.writeError(w, err)
	}
	if upgraded {
		return
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
	return ""
}

func dialDockerWrapper(string, string) (net.Conn, error) {
	return dialDocker()
}

func dialDocker() (net.Conn, error) {
	return net.Dial("unix", "/var/run/docker.sock")
}

func (p *dockerProxy) logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func (p *dockerProxy) logMessage(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, format, args...)
}

func (p *dockerProxy) tryUpgrade(w http.ResponseWriter, req *http.Request) (bool, error) {
	if !isUpgradeRequest(req) {
		return false, nil
	}
	backendConn, err := dialDocker()
	if err != nil {
		return true, err
	}
	defer backendConn.Close()

	requestHijackedConn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return true, err
	}
	defer requestHijackedConn.Close()

	newRequest, err := http.NewRequest(req.Method, p.dockerURL(req), req.Body)
	if err != nil {
		return true, err
	}
	newRequest.Header = req.Header

	if err = newRequest.Write(backendConn); err != nil {
		return true, err
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		_, err := io.Copy(backendConn, requestHijackedConn)
		if err != nil {
			p.logError("Error proxying data from client to backend: %v", err)
		}
		wg.Done()
	}()

	go func() {
		_, err := io.Copy(requestHijackedConn, backendConn)
		if err != nil {
			p.logError("Error proxying data from backend to client: %v", err)
		}
		wg.Done()
	}()

	wg.Wait()
	return true, nil
}
