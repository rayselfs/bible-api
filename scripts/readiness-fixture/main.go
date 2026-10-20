// A test-owned listener models a port conflict without touching any real service.
package main

import (
	"log"
	"net"
	"net/http"
	"sync"
)

func main() {
	held, err := net.Listen("tcp", ":50002")
	if err != nil {
		log.Fatal(err)
	}
	var once sync.Once
	http.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		once.Do(func() { held.Close() })
		w.WriteHeader(204)
	})
	log.Fatal(http.ListenAndServe(":8081", nil))
}
