package controller

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
	"go.dedis.ch/cs438/gui/httpnode/types"
	"go.dedis.ch/cs438/peer"

	"golang.org/x/net/html"
)

type redirectServer struct {
	peer      peer.Peer
	log       *zerolog.Logger
	isRunning bool
}

const localPort = ":8080"
const hostURL = "http://localhost" + localPort + "/"
const error404File = "404.html"

func NewRedirectServer(peer peer.Peer, log *zerolog.Logger) redirectServer {
	return redirectServer{
		peer: peer,
		log:  log,
	}
}

func (re *redirectServer) BrowseHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Access-Control-Allow-Origin", "*")
			re.startRedirectServer()
			re.respondRedirectUrl(w, r)
		case http.MethodOptions:
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "*")
		default:
			http.Error(w, "forbidden method", http.StatusMethodNotAllowed)
		}
	}
}

// Start server to listen for redirect website
func (re *redirectServer) startRedirectServer() {
	if !re.isRunning {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, re.getWebsiteAnd(r.URL.Path[1:], func(p string) {
				decorateFolder(p)
			}))
		})
		re.log.Info().Msg("Starting server at port " + localPort + "\n")
		go http.ListenAndServe(localPort, nil)
		re.isRunning = true
	}
}

func (re *redirectServer) respondRedirectUrl(w http.ResponseWriter, r *http.Request) {
	resp := make(map[string]string)
	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body: "+err.Error(), http.StatusInternalServerError)
		return
	}
	res := types.BrowseRequest{}
	err = json.Unmarshal(buf, &res)
	if err != nil {
		http.Error(w, "failed to unmarshal browseWebsite: "+err.Error(),
			http.StatusInternalServerError)
		return
	}
	resp["redirect"] = hostURL + res.WebsiteName
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		re.log.Fatal().Msgf("Error happened in JSON marshal. Err: %s", err)
	}
	w.Write(jsonResp)
}

// Get files remotely from peerster and apply "and" function to local folder
func (r *redirectServer) getWebsiteAnd(websiteName string, and func(string)) string {
	addr := r.peer.Resolve(websiteName)
	fetchedRecord, ok := r.peer.FetchPointerRecord(addr)
	if !ok {
		return error404File
	}
	r.log.Info().Msg("fetched record: " + fetchedRecord.Value)
	res, err := r.peer.DownloadDHT(fetchedRecord.Value)
	if err != nil {
		return error404File
	}
	err = ioutil.WriteFile(websiteName+".html", res, 0664) // TODO handle folder and not only unique files
	if err != nil {
		return error404File
	}
	and(websiteName)
	return websiteName
}

// Decorate all HTML file in a folder by changing link to localhost redirect
func decorateFolder(path string) error {
	var files []string
	err := filepath.Walk(path,
		func(path string, info os.FileInfo, err error) error {
			if strings.HasSuffix(strings.ToLower(info.Name()), ".html") {
				files = append(files, path)
			}
			return err
		})
	if err != nil {
		return err
	}
	for _, f := range files {
		decorateHTML(f)
	}
	return nil
}

// Decorate the HTML file at path with redirected links to localhost
func decorateHTML(path string) error {
	text, err := fileToString(path)
	if err != nil {
		return err
	}
	links, err := extractHrefFromContent(text)
	if err != nil {
		return err
	}
	return stringToFile(path, replaceLinks(text, links))
}

func fileToString(path string) (string, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func stringToFile(path string, content string) error {
	return ioutil.WriteFile(path, []byte(content), 0644)
}

// Replace all links (currentUrls) in inputContent with localhost redirected one
func replaceLinks(inputContent string, currentUrls []string) string {
	var replaceArr []string
	for _, c := range currentUrls {
		replaceArr = append(replaceArr, c)
		if c[:5] == "https" {
			replaceArr = append(replaceArr, hostURL+c[8:])
		} else if c[:4] == "http" {
			replaceArr = append(replaceArr, hostURL+c[7:])
		} else {
			replaceArr = append(replaceArr, c)
		}
	}
	r := strings.NewReplacer(replaceArr...)
	return r.Replace(inputContent)
}

// Get the url from an html token
func getHrefFromToken(t html.Token) (ok bool, href string) {
	for _, a := range t.Attr {
		if a.Key == "href" {
			href = a.Val
			ok = true
		}
	}
	return
}

// Get all urls from an html file content
func extractHrefFromContent(content string) ([]string, error) {
	var results []string
	z := html.NewTokenizer(strings.NewReader(content))
	for {
		tt := z.Next()
		switch {
		case tt == html.ErrorToken:
			return results, nil
		case tt == html.StartTagToken:
			t := z.Token()
			if t.Data != "a" {
				continue
			}
			ok, url := getHrefFromToken(t)
			if !ok {
				continue
			}
			results = append(results, url)
		}
	}
}