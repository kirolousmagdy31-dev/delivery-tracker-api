package main

import (
	"net/http"

	"package-tracking/internal/common"
	"package-tracking/internal/courier"
	"package-tracking/internal/order"
	"package-tracking/internal/user"
)

func main() {
	db := common.OpenDB()
	mux := http.NewServeMux()

	user.Register(mux, db)    // Features #1, #2
	order.Register(mux, db)   // Features #3-#6, #9
	courier.Register(mux, db) // Features #7, #8, #10

	common.Serve("package-tracking-api", common.Env("PORT", "8080"), mux)
}
