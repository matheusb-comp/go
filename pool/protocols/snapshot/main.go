package snapshot

// Data structure for the main page JSON
type Digest struct {
	Pool string 	`json:"address"`
	Voters uint64 `json:"voters"`
	Votes uint64	`json:"votes"`
}

// Data structures for mapping the voters JSON
type Data struct {
	Name string		`json:"dataname"`
	Value string	`json:"datavalue"`
}
type Entry struct {
	ID	string		`json:"account"`
	Bal	uint64		`json:"balance"`
	Data []Data		`json:"data"`
}
type VoterList struct {
	Des string			`json:"inflationdest"`
	Entries	[]Entry	`json:"entries"`
}
