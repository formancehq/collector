package main

import (
	"context"
	"github.com/Jeffail/benthos/v3/public/service"
	_ "github.com/numary/organization-collector/pkg"
)

func main() {
	service.RunCLI(context.Background())
}
