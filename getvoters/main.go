package main

import (
	"log"
	"flag"
	"errors"
	"strconv"
	"net/http"
	"database/sql"
	"encoding/json"
	"encoding/base64"
	_ "github.com/lib/pq"
)

const VOTERS_QUERY = `SELECT accounts.accountid, balance, dataname, datavalue
FROM accounts LEFT JOIN accountdata
ON accountdata.accountid = accounts.accountid
AND dataname LIKE $1 WHERE inflationdest = $2`

const VOTERS_NUMBER_QUERY = `SELECT COUNT(accountid)
FROM accounts WHERE inflationdest = $1`

const TOTAL_VOTES_QUERY = `SELECT SUM(balance)
FROM accounts WHERE inflationdest = $1`

// JSON indent strings (https://golang.org/pkg/encoding/json/#Encoder.SetIndent)
const JSON_INDENT_PREFIX = ""
const JSON_INDENT_INDENT = ""

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

// Data structures to work internally (quicker access)
type VoterData struct {
	Balance uint64
	Data map[string]string
}
type Voters map[string]*VoterData

// User-defined variables
var dbUser, dbPass, dbName, dbHost, dbPort, dbConn string
var listenAddr, urlTotals, urlVoters, urlParam string
var defaultPool, donationKey string
var db *sql.DB

func init() {
	// Database flags
	flag.StringVar(&dbUser, "user", "stellar",
		"PostgreSQL user name to connect as")

	flag.StringVar(&dbPass, "pass", "",
		"Password to be used if the server demands password authentication")

	flag.StringVar(&dbName, "db", "core", "The database name")

	flag.StringVar(&dbHost, "host", "localhost",
		"Name of host to connect to. If a host name begins with a slash, " +
		"it specifies Unix-domain communication rather than TCP/IP communication")

	flag.StringVar(&dbPort, "port", "5432",
		"Port number to connect to at the server host, " +
		"or socket file name extension for Unix-domain connections")

	flag.StringVar(&dbConn, "conn", "",
		"Optional custom PostgreSQL connection string. If provided, " +
		"it's used instead of the other flags")

	// Server flags
	flag.StringVar(&listenAddr, "listen", "0.0.0.0:8080",
		"Address (host:port) to listen for requests")

	flag.StringVar(&urlTotals, "totals", "/totals",
		"URL pattern in the default HTTP request multiplexer to get the totals")

	flag.StringVar(&urlVoters, "voters", "/voters",
		"URL pattern in the default HTTP request multiplexer to get the voters list")

	flag.StringVar(&urlParam, "param", "pool",
		"Parameter to expect in the HTTP GET request URL (example: <URL>?pool=<ADDR>)")

	// Stellar flags
	flag.StringVar(&defaultPool, "pool",
		"GCCD6AJOYZCUAQLX32ZJF2MKFFAUJ53PVCFQI3RHWKL3V47QYE2BNAUT",
		"Default inflationdest address to use")

	flag.StringVar(&donationKey, "key", "lumenaut.net donation%",
		"Format of key for a voter data pair to mark a donation")
}

func main() {
	flag.Parse()

	// Create a connection string only if one was not supplied
	conn := dbConn
	if len(conn) < 1 {
		conn = "dbname=" + dbName +
		" user=" + dbUser +
		" password=" + dbPass +
		" host=" + dbHost +
		" port=" + dbPort
	}

	// Try opening the connection with the DB
	var err error
	db, err = sql.Open("postgres", conn)
	if err != nil {
		log.Fatal("ERROR opening DB connection: " + err.Error())
	}
	defer db.Close()

	// Set the function to handle requests and start server
	http.HandleFunc(urlTotals, getTotals)
	http.HandleFunc(urlVoters, getVoters)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}

func poolFromParam(r *http.Request) string {
	// Ignore the address if it doesn't have 56 characters and starts wiht a G
	pool := r.URL.Query().Get(urlParam)
	if len(pool) != len(defaultPool) || pool[0] != 'G' {
		pool = defaultPool
	}
	return pool
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	// Inform the user of the content-type in the header
	w.Header().Set("Content-Type", "application/json")

	// Set up the compressing/encoding pipeline
	js := json.NewEncoder(w)

	// Set the JSON indentation strings
	js.SetIndent(JSON_INDENT_PREFIX, JSON_INDENT_INDENT)

	// Marshal data as JSON and send to w
	err := js.Encode(data)
	if err != nil {
		log.Println("ERROR writing JSON/gzip-encoded response: " + err.Error())
	}
}

func getTotals(w http.ResponseWriter, r *http.Request) {
	log.Println(
		"Request: " + r.Method + " " + r.URL.String() +
		" - From: " + r.RemoteAddr)

	// Values for the total number of voters and sum of votes
	var voters, votes uint64
	pool := poolFromParam(r)

	// QueryRow executes a query that is expected to return at most one row
	err := db.QueryRow(VOTERS_NUMBER_QUERY, pool).Scan(&voters)
	if err != nil {
		log.Println("ERROR getting the number of voters: " + err.Error())
		http.Error(w, "500 internal server error", 500)
		return
	}
	err = db.QueryRow(TOTAL_VOTES_QUERY, pool).Scan(&votes)
	if err != nil {
		log.Println("ERROR getting the total of votes: " + err.Error())
		http.Error(w, "500 internal server error", 500)
		return
	}

	writeJSON(w, &Digest{Pool: pool, Voters: voters, Votes: votes})
}

func getVoters(w http.ResponseWriter, r *http.Request) {
	log.Println(
		"Request: " + r.Method + " " + r.URL.String() +
		" - From: " + r.RemoteAddr)

	pool := poolFromParam(r)

	// Get the voters map from the DB
	voters, err := getVotersDB(pool)
	if err != nil {
		log.Println(err)
		http.Error(w, "500 internal server error", 500)
		return
	}

	// Create the structure that can be mapped to JSON
	var vl VoterList
	vl.Des = pool

	// Loop all voters and fill up the VoterList
	for key, value := range voters {
		entry := Entry{ID: key, Bal: value.Balance}
		// Loop all the data for this voter (can be nil)
		for k, v := range value.Data {
			data := Data{Name: k, Value: v}
			entry.Data = append(entry.Data, data)
		}
		// Append this voter (and data) to the list
		vl.Entries = append(vl.Entries, entry)
	}

	writeJSON(w, &vl)
}

func getVotersDB(pool string) (Voters, error) {
	// Try executing the query
	rows, err := db.Query(VOTERS_QUERY, donationKey, pool)
	if err != nil {
		return nil, errors.New("ERROR executing query: " + err.Error())
	}
	defer rows.Close()

	// Create the map to receive DB data
	vl := make(Voters)

	// Loop all the rows (IDs and balances can repeat, name and value can be null)
	for rows.Next() {
		var balance uint64
		var id, bstring string
		var nameNull, valueNull sql.NullString

		// Copies the columns in the current row into the parameters
		err = rows.Scan(&id, &bstring, &nameNull, &valueNull)
		if err != nil {
			return nil, errors.New("ERROR scanning query result row: " + err.Error())
		}

		// Convert the string balance to integer
		balance, err := strconv.ParseUint(bstring, 10, 64)
		if err != nil {
			return nil, errors.New("ERROR parsing balance string: " + err.Error())
		}

		// Make sure this voter exist in the map, or create a new one
		if v := vl[id]; v == nil {
			vl[id] = new(VoterData)
		}

		// Add this voter's balance to the structure (update if repeated)
		vl[id].Balance = balance

		// Add the (key, value) pair, if it exists
		if nameNull.Valid && valueNull.Valid {
			// We expect a base64 encoded string
			decoded, err := base64.StdEncoding.DecodeString(valueNull.String)
			// If err != nil, this data is simply ignored
			if err == nil {
				// Adding data to a uninitialized map is a runtime panic
				if vl[id].Data == nil {
					vl[id].Data = make(map[string]string)
				}
				// Finally, add the data to the structure
				vl[id].Data[nameNull.String] = string(decoded)
			}
		}
	}

	// Get any error encountered during iteration
	err = rows.Err()
	if err != nil {
		return nil, errors.New("ERROR iterating query results: " + err.Error())
	}

	// Return the structure
	return vl, nil
}
