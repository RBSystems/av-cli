package float

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/byuoitav/av-cli/cmd/args"
	"github.com/byuoitav/av-cli/cmd/wso2"
	"github.com/cheggaaa/pb"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

// shipCmd .
var shipCmd = &cobra.Command{
	Use:   "ship [device ID]",
	Short: "Deploys to the device with the given ID",
	Args:  args.ValidDeviceID,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Deploying to %s\n", args[0])

		dbPrompt := promptui.Select{
			Label: "Database to deploy from",
			Items: []string{"development", "stage", "production"},
		}

		_, result, err := dbPrompt.Run()
		if err != nil {
			fmt.Printf("prompt failed %v\n", err)
		}

		fmt.Printf("result: %s\n", result)
		bar := pb.New(6).SetWidth(50).Format(fmt.Sprintf("%s [\x00=\x00>\x00-\x00]", args[0]))
		bar.ShowCounters = false
		bar.Start()
		err = floatshipWithBar(args[0], result, bar)
		if err != nil {
			fmt.Printf("Error floating ship: %v\n", err)
			return
		}

	},
}

func floatship(deviceID, designation string) error {
	var dbDesignation string
	switch designation {
	case "development":
		dbDesignation = "dev"
	case "stage":
		dbDesignation = "stg"
	case "production":
		dbDesignation = "prd"
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.byu.edu/domains/av/flight-deck/%v/webhook_device/%v", dbDesignation, deviceID), nil)
	if err != nil {
		return fmt.Errorf("couldn't make request: %v", err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %v", wso2.GetAccessToken()))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't perform request: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("non-200 status code: %v", resp.StatusCode)
	}

	fmt.Printf("Deployment successful\n")
	return nil
}

func floatshipWithBar(deviceID, designation string, bar *pb.ProgressBar) error {
	//1
	bar.Increment()

	var dbDesignation string
	switch designation {
	case "development":
		dbDesignation = "dev"
	case "stage":
		dbDesignation = "stg"
	case "production":
		dbDesignation = "prd"
	}

	//2
	bar.Increment()

	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.byu.edu/domains/av/flight-deck/%v/webhook_device/%v", dbDesignation, deviceID), nil)
	if err != nil {
		return fmt.Errorf("couldn't make request: %v", err)
	}

	//3
	bar.Increment()

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %v", wso2.GetAccessToken()))

	//4
	bar.Increment()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't perform request: %v", err)
	}
	//5
	bar.Increment()

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("couldn't read the response body: %v", err)
	}

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("non-200 status code: %v - %s", resp.StatusCode, body)
	}
	//6
	bar.Increment()
	bar.Finish()
	return nil
}
