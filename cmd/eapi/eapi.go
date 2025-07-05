package main

import (
	"os"

	"github.com/chenwei67/eapi"
	"github.com/chenwei67/eapi/plugins/echo"
	"github.com/chenwei67/eapi/plugins/gin"
)

func main() {
	eapi.NewEntrypoint(
		gin.NewPlugin(),
		echo.NewPlugin(),
	).Run(os.Args)
}
