package service

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/machinebox/graphql"
	"github.com/tokamak-network/DRB-Node/logger" // Import your logger package
	"github.com/tokamak-network/DRB-Node/utils"
)

func IsOperator(operator string) (bool, error) {
	config := utils.GetConfig()
	client := graphql.NewClient(config.SubgraphURL)

	req := utils.GetIsOperatorRequest(operator)

	var respData struct {
		OperatorNumberChangeds []utils.OperatorNumberChanged `json:"operatorNumberChangeds"`
	}

	ctx := context.Background()
	if err := client.Run(ctx, req, &respData); err != nil {
		logger.Log.Errorf("Failed to execute query for operator %s: %v", operator, err)
		return false, err
	}

	// Log the fetched data for debugging
	if len(respData.OperatorNumberChangeds) > 0 {
		logger.Log.Infof("Fetched %d operator records", len(respData.OperatorNumberChangeds))
		for _, record := range respData.OperatorNumberChangeds {
			logger.Log.Infof("Operator status: %v", record.IsOperator)
		}
	} else {
		logger.Log.Infof("No operator records found for %s", operator)
	}

	// Print results to the terminal
	fmt.Println("---------------------------------------------------------------------------")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.Debug)
	fmt.Fprintln(w, "Operator\tStatus")
	if len(respData.OperatorNumberChangeds) > 0 {
		for _, record := range respData.OperatorNumberChangeds {
			fmt.Fprintf(w, "%s\t%v\n", operator, record.IsOperator)
		}
	} else {
		fmt.Fprintf(w, "%s\tNo records found\n", operator)
	}
	w.Flush()
	fmt.Println("---------------------------------------------------------------------------")

	if len(respData.OperatorNumberChangeds) > 0 {
		return respData.OperatorNumberChangeds[0].IsOperator, nil
	}

	return false, nil
}
