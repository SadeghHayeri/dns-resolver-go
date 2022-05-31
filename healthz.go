package main

import (
	"fmt"
	"net/http"
)

func returnSuccess(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(200)
	fmt.Fprintf(w, "healthy!\n")
}

func RunHealthServer() {
	http.HandleFunc("/healthz", returnSuccess)
	http.ListenAndServe(":8090", nil)
}
