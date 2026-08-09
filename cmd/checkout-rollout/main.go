package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/infrai-examples/compliant-checkout-rollout/internal/infrai"
)

func main() {
	flagKey := flag.String("flag", "checkout_v2", "e-commerce feature flag key")
	percentage := flag.Int("percent", 5, "customer rollout percentage")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client, err := infrai.NewClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	version, err := client.SetBoolean(ctx, *flagKey, false)
	if err != nil {
		log.Fatal(err)
	}
	if err := client.Rollout(ctx, *flagKey, *percentage, version); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s rollout accepted: %d%%\n", *flagKey, *percentage)
}
