package helpers

import (
	"fmt"
	"os/user"

	"github.com/pocketbase/pocketbase"
)

type Helpers struct {
	app *pocketbase.PocketBase
}

func New(app *pocketbase.PocketBase) *Helpers {
	return &Helpers{app: app}
}

func (h *Helpers) GetHomeDir() string {
	currentUser, err := user.Current()
	if err != nil {
		fmt.Println(err)
		return ""
	}
	return currentUser.HomeDir
}
