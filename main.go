package main

import (
	"crypto/rand"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"strconv"

	"github.com/boltdb/bolt"
	"github.com/gorilla/mux"
)

const (
	AUTHED_CLIENTS_BUCKET = "authed_clients"
	TOKEN_BYTES_LENGTH    = 32
)

type AuthClient struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Gun struct {
	ammoFile string
	bolt     *bolt.DB
	users    []AuthClient
	router   *mux.Router
}

func openTextFile(path string) []byte {
	fileBytes, err := ioutil.ReadFile(path)
	if err != nil {
		panic("Could not read file: " + path + "\nExiting...")
	}
	return fileBytes
}

func createAuthBucket(db *bolt.DB) {
	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(AUTHED_CLIENTS_BUCKET))
		if err != nil {
			return fmt.Errorf("Couldn't create auth clients bucket: %s", err)
		}
		return nil
	})
}

func jSONEqual(a string, b string) (bool, error) {
	var o1 interface{}
	var o2 interface{}

	var err error
	err = json.Unmarshal([]byte(a), &o1)
	if err != nil {
		log.Print(a)
		_, errorMsg := fmt.Printf("Error marshalling JSON A during comparison :: %s", err.Error())
		return false, errorMsg
	}
	err = json.Unmarshal([]byte(b), &o2)
	if err != nil {
		log.Print(b)
		_, errorMsg := fmt.Printf("Error marshalling JSON B during comparison :: %s", err.Error())
		return false, errorMsg
	}

	return reflect.DeepEqual(o1, o2), nil
}

func getAuthToken(db *bolt.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		keyFound := false
		body, bodyReadErr := ioutil.ReadAll(r.Body)
		if bodyReadErr != nil {
			log.Printf("Error reading request body: %v", bodyReadErr)
			http.Error(w, "Unable to read request body!", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		// Check to make sure that the body
		// can be read as an authclient
		var authClient AuthClient
		unmarshalErr := json.Unmarshal(body, &authClient)
		if unmarshalErr != nil {
			log.Printf("Error marshaling request body: %v", unmarshalErr)
			http.Error(w, "Invalid request body!", http.StatusBadRequest)
			return
		}
		// Search DB for matching key
		// If found, return as JSON
		// If not, return JSON error
		db.View(func(tx *bolt.Tx) error {
			// Assume bucket exists and has keys
			b := tx.Bucket([]byte(AUTHED_CLIENTS_BUCKET))
			c := b.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				// if json key = json client
				clientFound, comparisonError := jSONEqual(string(k), string(body))
				if comparisonError != nil {
					log.Fatal()
				}
				if clientFound {
					keyFound = true
					tokenResponse := struct {
						Token string `json:"token"`
					}{
						b64.StdEncoding.EncodeToString([]byte(v)),
					}
					response, marshalErr := json.Marshal(tokenResponse)
					if marshalErr != nil {
						http.Error(w, "{ \"error\": \"Unable to provide response!\" }", http.StatusInternalServerError)
					}
					fmt.Fprintln(w, string(response))
				}
			}
			return nil
		})
		if keyFound == false {
			errorResponse := struct {
				ErrorMsg string `json:"error"`
			}{
				"Unable to authenticate!",
			}
			response, marshalErr := json.Marshal(errorResponse)
			if marshalErr != nil {
				http.Error(w, "{ \"error\": \"Unable to provide response!\" }", http.StatusInternalServerError)
			}
			fmt.Fprintln(w, string(response))
		}
	}
}

func updateDB(db *bolt.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO
		// Check to see if bucket exists. If not, error
		// Update or create key in bucket
		// Return success
		// cursor.delete() see authenticate
	}
}

func retrieveDB(db *bolt.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO
		// Check to see if bucket exists. If not, error
		// Access key in bucket
		// Return as JSON
	}
}

func Init(ammoFilePath string, dbFilePath string) *Gun {
	// Open ammo file from directory
	ammoFileContents := openTextFile(ammoFilePath)
	// Parse JSON into AuthClients
	authenticatedClients := make([]AuthClient, 0)
	json.Unmarshal(ammoFileContents, &authenticatedClients)
	// Connect to / open db file
	db, err := bolt.Open(dbFilePath, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	// For each authed user / client, assign them an auth token if don't have one
	createAuthBucket(db)
	for i := range authenticatedClients {
		// Get key used to identify a client with their token
		clientKey, jsonErr := json.Marshal(authenticatedClients[i])
		if jsonErr != nil {
			log.Fatal("Error storing auth token in DB.")
		}
		db.Update(func(tx *bolt.Tx) error {
			// Open the auth bucket
			b := tx.Bucket([]byte(AUTHED_CLIENTS_BUCKET))
			// Generate a client token to be associated with the key
			clientToken := make([]byte, TOKEN_BYTES_LENGTH)
			_, tokenErr := io.ReadFull(rand.Reader, clientToken)
			if tokenErr != nil {
				log.Fatal("Error generating token.")
			}
			// Store the key with its associated token
			txionErr := b.Put(clientKey, clientToken)
			return txionErr
		})
	}
	// Create router
	router := mux.NewRouter()
	router.HandleFunc("/authenticate", getAuthToken(db)).
		Methods("POST")
	router.HandleFunc("/{bucket}/update", updateDB(db)).
		Methods("POST")
	router.HandleFunc("/{bucket}/retrieve", retrieveDB(db)).
		Methods("GET")
	// Init Boltgun instance
	return &Gun{
		ammoFilePath,
		db,
		authenticatedClients,
		router,
	}
}

func (g *Gun) Fire(port int64) {
	actualPort := ":" + strconv.FormatInt(port, 10)
	http.ListenAndServe(actualPort, g.router)
	defer g.bolt.Close()
}