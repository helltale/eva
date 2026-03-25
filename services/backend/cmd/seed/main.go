package main

import (
	"context"
	"fmt"
	"os"

	"eva/services/backend/internal/config"
	"eva/services/backend/internal/migrate"
	"eva/services/backend/internal/repository"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx := context.Background()
	if err := migrate.Up(ctx, cfg.PostgresDSN); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	st, err := repository.NewStore(ctx, cfg.PostgresDSN)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer st.Close()
	n, err := st.UserCount(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if n > 0 {
		fmt.Println("users already exist, skip seed")
		return
	}
	u, err := st.CreateUser(ctx, "admin@local", "changeme")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = st.EnsureSettings(ctx, u.ID)
	fmt.Printf("created user %s id=%s password=changeme\n", u.EmailOrUsername, u.ID)
}
