package main

// TODO: write a markdown parser that preverse semantics better

import (
	"log"
	"net/http"
)

func main() {
	fs := http.FileServer(http.Dir("./build"))
	http.Handle("/", fs)

	log.Println("Listening on :8080")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
