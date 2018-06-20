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

// [DEPRECATED] Function to substitute the HAL templated URI (RFC 6570)
// Using now a more general library github.com/jtacoma/uritemplates
// func convert(s string, cur string, lim int, asc bool) string {
//   // Set the URL query parameters (?arg1=v1&arg2=v2...)
//   param := "?order="
//   if asc {
//     param += "asc&"
//   } else {
//     param += "desc&"
//   }
//   if lim > 0 {
//     param += "limit=" + strconv.Itoa(lim) + "&"
//   }
//   if cur != "" {
//     param += "cursor=" + cur
//   } else {
//     // Remove trailing '&'
//     param = string(param[:len(param)-1])
//   }
//   // Regular expression to get the template ({?cursor,limit,order})
//   re := regexp.MustCompile("\\{\\?[a-z,]+\\}")
//   // Replace the template with the set URL query parameters
// 	return re.ReplaceAllLiteralString(s, param)
// }
