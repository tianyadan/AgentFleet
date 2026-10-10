package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"atolla/server/config"
	"atolla/server/internal/auth"
	"atolla/server/internal/handler"
	"atolla/server/internal/service"
	"atolla/server/internal/store"
)

func main() {
	cfg := config.Load()

	st, err := store.New(cfg.DBDSN)
	if err != nil {
		log.Fatalf("connect mysql: %v", err)
	}

	svc := service.New(cfg, st)
	h := handler.New(svc)

	r := gin.Default()
	if len(cfg.AllowedIPs) > 0 {
		r.Use(auth.GinIPAllowlist(cfg.AllowedIPs))
	}
	h.Register(r)

	log.Printf("atolla listening on %s", cfg.Addr)
	if err := r.Run(cfg.Addr); err != nil {
		svc.Perms.CloseAll()
		svc.Sched.Stop()
		log.Fatal(err)
	}
	svc.Perms.CloseAll()
	svc.Sched.Stop()
}
