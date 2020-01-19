package api

import (
	"html/template"
	"net/http"
)

func html(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		t, _ := template.ParseFiles("home.html")
		t.Execute(w, nil)
	}
}
