package app


import (
	"log"
	"net/http"
	"net/http/httputil"
	"net"
	"strings"
)

func logRequest(req *http.Request, status int, reason string) {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}

	log.Printf("%s %s %s %d \"%s\"\n", host, req.Method, req.RequestURI, http.StatusForbidden, reason)
}

func Forward(w http.ResponseWriter, req *http.Request) {
	token := ""
	if cookie, err := req.Cookie("access_token"); err == nil {
		token = cookie.Value
	}
	if !LookupAccess(token, req.URL.Path) {
		logRequest(req, http.StatusForbidden, "Invalid access token")
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}
	info := LookupPath(req.URL.Path)
	if info == nil {
		logRequest(req, http.StatusNotFound, "Path mapping not found")
		http.Error(w, "Path not mapped", http.StatusNotFound)
		return
	}

	sshClient := getSSHConnection(&info.Server)
	if sshClient == nil {
		logRequest(req, http.StatusInternalServerError, "Couldn't connect to SSH server")
		return
	}

	var revProxy http.Handler
	director := func(req *http.Request) {
		req.URL.Path = info.HttpProxy.BasePath + strings.TrimPrefix(req.URL.Path, info.Prefix)
		req.URL.Scheme = "http"
		req.URL.Host = info.HttpProxy.Address
	}

	if (isWebsocket(req)) {
		revProxy = &WebsocketReverseProxy{
			Director: director,
			Dial: func(network, addr string) (net.Conn, error) {
				log.Println(`SSH->WebSocket @ ` + info.HttpProxy.Address)
				return sshClient.Dial(`tcp`, addr)
			},
		}

	} else {
		revProxy = &httputil.ReverseProxy{
			Director: director,
			Transport: &http.Transport{
				Dial: func(network, addr string) (net.Conn, error) {
					log.Println(`SSH->HTTP @ ` + info.HttpProxy.Address)
					return sshClient.Dial(`tcp`, addr)
				},
			},
		}
	}
	revProxy.ServeHTTP(w, req)
}
