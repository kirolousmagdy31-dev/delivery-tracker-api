package main

import (
	"net/http"

	"package-tracking/internal/common"
	"package-tracking/internal/order"
	"package-tracking/internal/user"
)

func main() {
	db := common.OpenDB()
	mux := http.NewServeMux()

	user.Register(mux, db)
	order.Register(mux, db)

	common.Serve("package-tracking-api", common.Env("PORT", "8080"), mux)
}