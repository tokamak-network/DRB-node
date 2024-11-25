package utils

import "github.com/machinebox/graphql"

const (
    // GraphQL query to fetch the rounds information
    RoundsQuery = `
		query MyQuery {
			rounds(where: {randomNumberGenerated: null}) {
				merkleRootSubmitted {
					merkleRoot
				}
				round
				randomNumberGenerated {
					randomNumber
				}
				randomNumberRequested {
					activatedOperators
				}
			}
		}`

    GetActivatedOperatorsAtRoundQuery = `
        query MyQuery($round: Int!) {
            randomNumberRequesteds(where: {round: $round}) {
                activatedOperators
        }
    `
)

// GetRoundsRequest returns a GraphQL request for fetching rounds.
func GetRoundsRequest() * graphql.Request {
    return graphql.NewRequest(RoundsQuery)
}

func GetActivatedOperatorsAtRoundRequest(round string) *graphql.Request {
	req := graphql.NewRequest(GetActivatedOperatorsAtRoundQuery)
	req.Var("round", round)
	return req
}