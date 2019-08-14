package swab

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/byuoitav/av-cli/cmd/args"
	"github.com/byuoitav/av-cli/cmd/pi"
	"github.com/spf13/cobra"
)

func init() {
	Cmd.AddCommand(swabRoomCmd)
	Cmd.AddCommand(swabBuildingCmd)
}

// Cmd .
var Cmd = &cobra.Command{
	Use:   "swab [device ID]",
	Short: "Refreshes the database/ui of a pi",
	Long:  "Forces a replication of the couch database, and causes the ui to refresh shortly after",
	Args:  args.ValidDeviceID,
	Run: func(cmd *cobra.Command, arg []string) {
		fmt.Printf("Swabbing %s\n", arg[0])

		db, _, err := args.GetDB()
		if err != nil {
			fmt.Printf("unable to get database: %v", err)
			os.Exit(1)
		}

		// look it up in the database
		device, err := db.GetDevice(arg[0])
		if err != nil {
			fmt.Printf("unable to get device from database: %s\n", err)
			os.Exit(1)
		}

		err = swab(context.TODO(), device.Address)
		if err != nil {
			fmt.Printf("unable to swab %s: %s\n", device.ID, err)
			os.Exit(1)
		}

		fmt.Printf("Successfully swabbed %s\n", device.ID)
	},
}

func swab(ctx context.Context, address string) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:7012/replication/start", address), nil)
	if err != nil {
		return fmt.Errorf("unable to build replication request: %s", err)
	}

	req = req.WithContext(ctx)

	// TODO actually validate that it worked
	_, err = http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("unable to start replication: %s", err)
	}

	fmt.Printf("%s\tReplication started\n", address)
	time.Sleep(3 * time.Second) // TODO make sure this doesn't overrun ctx

	req, err = http.NewRequest("PUT", fmt.Sprintf("http://%s:8888/refresh", address), nil)
	if err != nil {
		return fmt.Errorf("unable to build refresh request: %s", err)
	}

	req = req.WithContext(ctx)

	_, err = http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("unable to start replication: %s", err)
	}

	fmt.Printf("%s\tUI refreshed\n", address)

	client, err := pi.NewSSHClient(address)
	if err != nil {
		fmt.Printf("unable to ssh into %s: %s", address, err)
		os.Exit(1)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		fmt.Printf("unable to start new session: %s", err)
		client.Close()
		os.Exit(1)
	}

	fmt.Printf("Restarting DMM...\n")

	bytes, err := session.CombinedOutput("sudo systemctl restart device-monitoring.service")
	if err != nil {
		fmt.Printf("unable to reboot: %s\noutput on pi:\n%s\n", err, bytes)
		client.Close()
		os.Exit(1)
	}
	client.Close()
	fmt.Printf("Success\n")

	return nil
}
