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

	CommitCsQuery = `
        query MyQuery($round: String!, $msgSender: String!) {
            commitCs(where: {round: $round, msgSender: $msgSender}) {
                blockTimestamp
                commitVal
            }
        }`

	RecoveredDataQuery = `
        query MyQuery($round: String!) {
            recovereds(orderBy: blockTimestamp, orderDirection: asc, where: {round: $round}) {
                round
                blockTimestamp
                id
                msgSender
                omega
                roundInfo {
                    isRecovered
                }
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

	FulfillRandomnessDataQuery = `
        query GetFulfillRandomness($round: String!) {
            fulfillRandomnesses(where: {round: $round}) {
                msgSender
                blockTimestamp
                success
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

// GetCommitCsRequest returns a GraphQL request for fetching commitCs data.
func GetCommitCsRequest(round, msgSender string) *graphql.Request {
	req := graphql.NewRequest(CommitCsQuery)
	req.Var("round", round)
	req.Var("msgSender", msgSender)
	return req
}

// GetRecoveredDataRequest returns a GraphQL request for fetching recovered data.
func GetRecoveredDataRequest(round string) *graphql.Request {
	req := graphql.NewRequest(RecoveredDataQuery)
	req.Var("round", round)
	return req
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

// GetFulfillRandomnessDataRequest returns a GraphQL request for fetching fulfill randomness data.
func GetFulfillRandomnessDataRequest(round string) *graphql.Request {
	req := graphql.NewRequest(FulfillRandomnessDataQuery)
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
