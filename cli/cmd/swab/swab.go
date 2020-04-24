package swab

import (
	"context"
	"fmt"
	"io"
	"os"

	avcli "github.com/byuoitav/av-cli"
	"github.com/byuoitav/av-cli/cli/cmd/args"
	"github.com/byuoitav/av-cli/cli/cmd/pi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Cmd .
var Cmd = &cobra.Command{
	Use:   "swab [ID]",
	Short: "Refreshes the database/ui of a pi/room/building",
	Long:  "Forces a replication of the couch database, and causes the ui to refresh shortly after",
	Args:  args.ValidID,
	Run: func(cmd *cobra.Command, arg []string) {
		fmt.Printf("Swabbing %s\n", arg[0])
		fail := func(format string, a ...interface{}) {
			fmt.Printf(format, a...)
			os.Exit(1)
		}

		conn, err := grpc.Dial(viper.GetString("api"), grpc.WithInsecure())
		if err != nil {
			fmt.Printf("error making grpc connection: %v", err)
			os.Exit(1)
		}

		cli := avcli.NewAvCliClient(conn)

		_, designation, err := args.GetDB()
		if err != nil {
			fmt.Printf("error getting designation: %v", err)
			os.Exit(1)
		}

		stream, err := cli.Swab(context.TODO(), &avcli.ID{Id: arg[0], Designation: designation})
		if err != nil {
			if s, ok := status.FromError(err); ok {
				switch s.Code() {
				case codes.Unavailable:
					fail("api is unavailable\n")
				default:
					fail("%s\n", s.Err())
				}
			}

			fail("unable to swab: %s\n", err)
		}

		for {
			in, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if in.Error != "" {
				fmt.Printf("there was an error swabbing %s: %s\n", in.Id, in.Error)
			} else {
				address := in.Id + ".byu.edu"
				client, err := pi.NewSSHClient(address)
				if err != nil {
					err = fmt.Errorf("unable to ssh into %s: %s", address, err)
					fmt.Printf(err.Error())
					continue
				}
				defer client.Close()

				session, err := client.NewSession()
				if err != nil {
					client.Close()
					err = fmt.Errorf("unable to start new session: %s", err)
					fmt.Printf(err.Error())
					continue
				}

				fmt.Printf("Restarting DMM... \n")

				bytes, err := session.CombinedOutput("sudo systemctl restart device-monitoring.service")
				if err != nil {
					client.Close()
					err = fmt.Errorf("unable to reboot: %s\noutput on pi: \n%s\n", err, bytes)
					fmt.Printf(err.Error())
					continue
				}
				client.Close()
				fmt.Printf("Successfully swabbed %s\n", in.Id)
			}
		}
	},
}
