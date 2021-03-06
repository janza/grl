package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os"

	"github.com/boltdb/bolt"
	"github.com/mvdan/xurls"
)

func getSchemeAndHost(r *http.Request) string {
	scheme := r.URL.Scheme
	if scheme == "" {
		scheme = r.Header.Get("X-Scheme")
	}
	if scheme == "" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

func main() {
	db, err := bolt.Open("grl.db", 0600, nil)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	t, err := template.New("index").Parse(`#!/usr/bin/env bash

# grl: command line url shortener.
#
# Requires: xsel, xclip or pbcopy
#
# Installation: curl '{{.}}' | sudo tee /usr/bin/grl && chmod +x /usr/bin/grl
#
# Source: https://github.com/janza/grl
#
# Example usage:
#     echo google.com | grl
#     grl /path/to/file

set -e

text=$(cat "${1:-/dev/stdin}")

if [[ $text == "" ]]; then
    >&2 echo "empty input"
    exit 1
fi

copy_to_clipboard() {
	if type "xsel" &> /dev/null; then
		clip="xsel -ib"
	elif type "xclip" &> /dev/null; then
		clip="xclip -sel clip"
	else
		clip="pbcopy"
	fi
	echo -n "$1" | $clip
}

url=$(curl -s -X POST '{{.}}' -d "$text")

copy_to_clipboard "$url"
echo "$url"
	`)

	bucketName := []byte("Urls")

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hostAndSchema := getSchemeAndHost(r)
		if r.Method == "POST" {
			var inputString bytes.Buffer
			_, err := inputString.ReadFrom(r.Body)
			if err != nil {
				http.Error(w, err.Error(), 500)
			}
			fmt.Fprint(w, xurls.Relaxed.ReplaceAllStringFunc(inputString.String(), func(u string) string {
				if len(u) < 60 {
					return u
				}
				var id uint64
				if u[:4] != "http" {
					u = "http://" + u
				}
				err := db.Update(func(tx *bolt.Tx) error {
					b := tx.Bucket(bucketName)
					id, _ = b.NextSequence()

					hexId := []byte(fmt.Sprintf("%x", id))

					err := b.Put(hexId, []byte(u))
					return err
				})
				if err != nil {
					http.Error(w, err.Error(), 500)
				}
				return fmt.Sprintf("%s/%x", hostAndSchema, id)
			}))

			return
		}

		if r.Method == "GET" {
			if r.URL.Path == "/" {
				w.Header().Set("Content-Type", "text/plain")
				err := t.Execute(w, hostAndSchema)
				if err != nil {
					http.Error(w, err.Error(), 500)
				}
				return
			}
			err := db.View(func(tx *bolt.Tx) error {
				b := tx.Bucket(bucketName)
				id := []byte(r.URL.Path[1:])
				v := b.Get(id)
				if v != nil {
					http.Redirect(w, r, string(v), http.StatusFound)
				} else {
					w.WriteHeader(http.StatusNotFound)
					fmt.Fprintf(w, "Not found: %s", id)
				}

				return nil
			})
			if err != nil {
				http.Error(w, err.Error(), 500)
			}
			return
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "Not Allowed")
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Print(http.ListenAndServe(":"+port, nil))

}
