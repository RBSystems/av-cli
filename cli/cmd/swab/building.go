package swab

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/byuoitav/av-cli/cli/cmd/args"
	"github.com/byuoitav/common/db"
	"github.com/spf13/cobra"
)

// swabBuildingCmd .
var swabBuildingCmd = &cobra.Command{
	Use:   "building [building ID]",
	Short: "Refreshes the database/ui of all the pi's in a building",
	Long:  "Forces a replication of the couch database, and causes the ui to refresh shortly after",
	Args:  args.ValidBuildingID,
	Run: func(cmd *cobra.Command, arg []string) {
		fmt.Printf("Swabbing %s\n", arg[0])

		db, _, err := args.GetDB()
		if err != nil {
			fmt.Printf("unable to get database: %v", err)
			os.Exit(1)
		}

		err = swabBuilding(context.TODO(), db, arg[0])
		if err != nil {
			fmt.Printf("Couldn't swab building: %v", err)
			os.Exit(1)
		}
		// look it up in the database

		fmt.Printf("Successfully swabbed the %s\n", arg[0])
	},
}

func swabBuilding(ctx context.Context, db db.DB, buildingID string) error {
	rooms, err := db.GetRoomsByBuilding(buildingID)
	if err != nil {
		return fmt.Errorf("unable to get rooms from database: %s", err)
	}

	if len(rooms) == 0 {
		return fmt.Errorf("no rooms found in %s", buildingID)
	}

	wg := sync.WaitGroup{}

	for i := range rooms {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()
			fmt.Printf("Swabbing %s\n", rooms[idx].ID)
			err := swabRoom(ctx, db, rooms[idx].ID)
			if err != nil {
				fmt.Printf("unable to swab %s: %s\n", rooms[idx].ID, err)
				return
			}

			fmt.Printf("Swabbed %s\n", rooms[idx].ID)
		}(i)
	}

	wg.Wait()
	return nil
}