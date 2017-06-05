package main

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"

	"github.com/boltdb/bolt"
)

func main() {
	db, err := bolt.Open("grl.db", 0600, nil)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	bucketName := []byte("Urls")

	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucket(bucketName)
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var url bytes.Buffer
			url.ReadFrom(r.Body)

			// var idBytes []byte
			var id uint64
			db.Update(func(tx *bolt.Tx) error {
				b := tx.Bucket(bucketName)
				id, _ = b.NextSequence()

				err := b.Put([]byte(strconv.Itoa(int(id))), url.Bytes())
				return err
			})

			fmt.Fprintf(w, "%s/%d", r.Host, int(id))
			return
		}
		if r.Method == "GET" {
			db.View(func(tx *bolt.Tx) error {
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
			return
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "Not Allowed")
	})

	fmt.Print(http.ListenAndServe(":8080", nil))

}
