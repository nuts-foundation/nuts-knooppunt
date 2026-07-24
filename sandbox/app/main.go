package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
	}
	log.Printf("gf-sandbox listening on :%s", port)
	if err := http.ListenAndServe(":"+port, NewMux()); err != nil {
		log.Fatal(err)
	}
}
