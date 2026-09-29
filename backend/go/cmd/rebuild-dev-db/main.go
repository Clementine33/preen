// Command rebuild-dev-db backs up and rebuilds only the repository's .dev_data database.
// Run it from backend/go, after stopping the development backend.
package main

import (
	"log"

	"github.com/onsei/organizer/backend/internal/app"
)

func main() {
	backup, err := app.RebuildDevelopmentDatabase()
	if err != nil {
		log.Fatalf("rebuild failed (backup=%s): %v", backup, err)
	}
	log.Printf("development database rebuilt (backup=%s)", backup)
}
