# BoltGun
**An HTTP interface for communicating with a BoltDB instance.**  
BoltDB is a key/value store, and BoltGun lets you interact with it via HTTP.  

![BoltGun](https://raw.githubusercontent.com/aunyks/boltgun/master/pistol.png)

### How It Works
The common workflow for interacting with BoltGun is as follows.
1. Send BoltGun the username and password of a trusted client (defined in a `.ammo` file).  
    *It'll send back a token that you can use to make.*
2. Send BoltGun a POST request telling it how you want to modify your database.  
    *It'll let you know whether the operation was successful.*

### Setup
1. Create the `.ammo` file.
The `.ammo` file is a JSON list of username-password pairs used to identify clients that can receive auth tokens from BoltGun. It is a requirement for BoltGun to run.  
```json
[
    {
        "username": "alpha",
        "password": "beta"
    },
    {
        "username": "gamma",
        "password": "secret-delta"
    }
]
```
2. Import into your application.  
```go
import "github.com/aunyks/boltgun"
```
3. Create a new BoltGun instance.  
Before you create a BoltGun server, you'll have to configure and initialize a new BoltGun. To do this, you call the `Init()` function. This function accepts two string parameters: the first is the path (relative or absolute) to the `.ammo` file, and the second is the path where BoltGun will store its database file. If the database file doesn't exist, a new file will be created.
```go
gun := Init("./.ammo", "./gun.db")
```
4. Launch the BoltGun instance.  
Now you can start a BoltGun server. To do this, call the `Fire()` function. It accepts two parameters: the first is an `int64` denoting the port number to which BoltGun should bind, and the second is an optional path to the database file to which BoltGun should back up. If you don't want to back up, simply give an empty string (`""`)
```go
gun.Fire(8080, "./rifle.db")
```

### Usage
Let's add a new key-value pair to a bucket.  

1. Send a POST request to the `/authenticate` route consisting of the username and password of a client that you'd like to authenticate (accepted clients can be found in the `.ammo` file). You'll receive back either an error response or an auth token to be used in subsequent requests.  
*Note: You only have to authenticate a client when an auth token is invalid or on first startup of the server.*  
**Send**
```json
{
	"username": "alpha",
	"password": "beta"
}
```
**Receive**
```json
{
    "token": "sTeNSXL2yqGBQaqO1Cpx8S5Oq/xhk36/fLAqVZZaHok="
}
```
2. Send a POST request to the `/update` route consisting of the auth token, the bucket in which to store the key-value pair, the key name, and a string value. You'll receive back either an error response or a success response.  
**Send**
```json
{
    "token": "sTeNSXL2yqGBQaqO1Cpx8S5Oq/xhk36/fLAqVZZaHok=",
    "bucket": "my_cool_bucket",
    "key": "hi",
    "value": "bye"
}
```
**Receive**
```json
{
    "success": true
}
```

### Extras
- BoltGun can optionally back up its database file to another location every two minutes.
