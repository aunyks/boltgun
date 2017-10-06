package main

func main() {
	gun := Init("./.ammo", "./gun.db")
	gun.Fire(8080)
}
