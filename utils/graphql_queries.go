package utils

import "github.com/machinebox/graphql"

const (
	RandomWordsRequestedQuery = `
        query MyQuery {
            roundInfos(orderBy: requestedTimestamp, orderDirection: desc, first: 50){
                id,
                round,
                commitCount,
                revealCount,
                requestedTimestamp,
                isRefunded
            }
        }`

	CommitDataQuery = `
        query MyQuery($round: String!) {
            commits(where: {round: $round}){
            id
            operator
            blockTimestamp
            round
         }
        }`

    RevealDataQuery = `
        query MyQuery($round: String!) {
            reveals(where: {round: $round}){
            id
            operator
            blockTimestamp
            round
        }
    }`

	IsOperatorQuery = `
		query MyQuery {
            activatedOperators_collection {
                operators
                operatorsCount
            }
            activatedOperators(id: "activatedOperators") {
                operators
                operatorsCount
            }
        }`

	RoundInfosQuery = `
		query MyQuery {
		  roundInfos {
			commitCount
			id
			isRefunded
			requestedTimestamp
			revealCount
			round
		  }
		}`
)

// GetRandomWordsRequestedRequest returns a GraphQL request for fetching random words requested.
func GetRandomWordsRequestedRequest() *graphql.Request {
	return graphql.NewRequest(RandomWordsRequestedQuery)
}

// GetCommitDataRequest returns a GraphQL request for fetching commit data.
func GetCommitDataRequest(round string) *graphql.Request {
	req := graphql.NewRequest(CommitDataQuery)
	req.Var("round", round)
	return req
}

// GetRevealDataRequest returns a GraphQL request for fetching commit data.
func GetRevealDataRequest(round string) *graphql.Request {
	req := graphql.NewRequest(RevealDataQuery)
	req.Var("round", round)
	return req
}

// GetIsOperatorRequest returns a GraphQL request for checking if an address is an operator.
func GetIsOperatorRequest() *graphql.Request {
	req := graphql.NewRequest(IsOperatorQuery)
	return req
}

func GetRoundInfos() *graphql.Request {
	req := graphql.NewRequest(RoundInfosQuery)
	return req
}
