package main

import (
	"sync"
)

func main() {
	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()
		// runWhatsapp()
	}()

	go func() {
		defer wg.Done()
		runXmpp()
	}()

	wg.Wait()
}
