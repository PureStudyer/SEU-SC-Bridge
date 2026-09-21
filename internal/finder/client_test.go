package finder

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/auth"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync"
	"testing"
)

func TestChunkUploadAndJSONDownload(t *testing.T) {
	var mu sync.Mutex
	files := map[string][]byte{}
	chunks := map[string][]byte{}
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing auth")
		}
		if strings.Contains(r.URL.RawQuery, "test-token") {
			t.Error("token leaked into URL")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/finder/v2/upload/global_upload":
			if e := r.ParseMultipartForm(20 << 20); e != nil {
				t.Error(e)
				return
			}
			defer r.MultipartForm.RemoveAll()
			f, _, e := r.FormFile("file")
			if e != nil {
				t.Error(e)
				return
			}
			b, _ := io.ReadAll(f)
			f.Close()
			name := r.FormValue("filename")
			chunks[name] = append(chunks[name], b...)
			count++
			if r.FormValue("chunkNumber") != r.FormValue("totalChunks") {
				io.WriteString(w, "1")
			} else {
				io.WriteString(w, `{"needMerge":"true","result":200,"timeStamp":123}`)
			}
		case "/finder/v2/upload/file_merge":
			name := r.URL.Query().Get("filename")
			files[path.Join(r.URL.Query().Get("jobPath"), name)] = chunks[name]
			io.WriteString(w, `{"success":true,"code":0}`)
		case "/finder/v2/files/getStat":
			p := r.URL.Query().Get("path")
			b, ok := files[p]
			if !ok {
				io.WriteString(w, `{"success":true,"code":0,"data":""}`)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "code": 0, "data": Entry{Filename: path.Base(p), Bytes: int64(len(b))}})
		case "/finder/v2/files/rename":
			var args map[string]string
			json.NewDecoder(r.Body).Decode(&args)
			files[args["newName"]] = files[args["oldName"]]
			delete(files, args["oldName"])
			io.WriteString(w, `{"success":true,"code":0}`)
		case "/finder/v2/files/download":
			w.Header().Set("Content-Disposition", `attachment; filename="payload.json"`)
			w.Write(files[r.URL.Query().Get("path")])
		default:
			t.Error("unexpected route", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, Tokens: auth.New(func(context.Context) (auth.Credentials, error) { return auth.Credentials{Token: "test-token"}, nil })}
	data := bytes.Repeat([]byte("a"), 11<<20)
	if e := c.Upload(context.Background(), "/home/payload.json", bytes.NewReader(data), int64(len(data))); e != nil {
		t.Fatal(e)
	}
	if count != 2 {
		t.Fatal("expected two chunks", count)
	}
	var downloaded bytes.Buffer
	if e := c.Download(context.Background(), "/home/payload.json", &downloaded); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(downloaded.Bytes(), data) {
		t.Fatal("download changed bytes")
	}
}
func TestAPIErrorWithoutSuccessField(t *testing.T) {
	res := &http.Response{Body: io.NopCloser(strings.NewReader(`{"code":51,"message":"The Path field is required","data":null}`))}
	if decode(res, nil) == nil {
		t.Fatal("error was treated as success")
	}
}
func TestAuthRetriesOnce(t *testing.T) {
	attempts := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { attempts++; w.WriteHeader(401) }))
	defer s.Close()
	c := &Client{BaseURL: s.URL, Tokens: auth.New(func(context.Context) (auth.Credentials, error) { return auth.Credentials{Token: "opaque"}, nil })}
	_, e := c.request(context.Background(), "GET", "/files/getStat", url.Values{"path": {"/test"}}, nil, "")
	if e == nil || attempts != 2 {
		t.Fatal(attempts, e)
	}
}
