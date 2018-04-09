package main

import (
	//"fmt"
	"log"
	"bytes"
	"strconv"
	"net/http"
	"database/sql"
	"encoding/json"
	_ "github.com/lib/pq"
)

const (
	DB_CONN = "postgresql://stellar:psqlpass@localhost:5432/core"
	DB_USER = "stellar"
	DB_PASSWORD = "psqlpass"
	DB_NAME = "livecore"
	DB_QUERY = "SELECT accountid, balance FROM accounts WHERE inflationdest = $1"
	POOL = "GCCD6AJOYZCUAQLX32ZJF2MKFFAUJ53PVCFQI3RHWKL3V47QYE2BNAUT"
)

type Entry struct {
	ID	string	`json:"account"`
	Bal	string	`json:"balance"`
}

type VoterList struct {
	//Num	int		`json:"numvoters"`
	//Tot	int		`json:"totalvotes"`
	Dest	string	`json:"inflationdest"`
	Entries	[]Entry	`json:"entries"`
}

func main() {
	http.HandleFunc("/voters", getVoters)
	log.Fatal(http.ListenAndServe(":8000", nil))
}

func getVoters(w http.ResponseWriter, r *http.Request) {
	log.Println("Got a " + r.Method + " request")
	
	vl := getVotersDB()
	js, err := json.Marshal(vl)
	if err != nil {
		log.Fatal(err)
	}
	
	var b bytes.Buffer
	b.Write(js)
	
	n, err := b.WriteTo(w)
	if err != nil {
		log.Fatal(err)
	}
	
	log.Println(strconv.FormatInt(n, 10) + " bytes returned!")
}

func getVotersDB() VoterList {
	db, err := sql.Open("postgres", "user=" + DB_USER + " dbname=" + DB_NAME)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	
	rows, err := db.Query(DB_QUERY, POOL)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	
	// Create structure to receive the DB data
	var vl VoterList
	vl.Dest = POOL
	
	for rows.Next() {
		var id string
		var balance string
		
		err = rows.Scan(&id, &balance)
		if err != nil {
			log.Fatal(err)
		}
		
		// Add the row data to the struct
		/*vl.numvoters++
		bint, err := strconv.Atoi(balance)
		if err != nil {
			log.Fatal(err)
		}
		vl.totalvotes += bint*/
		var e = Entry{id, balance}
		vl.Entries = append(vl.Entries, e)
	}
	
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}
	
	return vl
}
