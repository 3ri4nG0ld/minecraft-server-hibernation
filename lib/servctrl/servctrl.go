package servctrl

import (
	"fmt"
	"sync/atomic"
	"time"

	"msh/lib/config"
	"msh/lib/logger"
)

// StartMS starts the minecraft server
func StartMS() error {
	// start server terminal
	err := cmdStart(config.ConfigRuntime.Server.Folder, config.ConfigRuntime.Commands.StartServer)
	if err != nil {
		return fmt.Errorf("StartMS: error starting minecraft server: %v", err)
	}

	return nil
}

// StopMS stops the minecraft server.
// When playersCheck == true, it checks for StopMSRequests/Players and orders the server shutdown
func StopMS(playersCheck bool) error {
	// error that returns from Execute() when executing the stop command
	var errExec error

	// wait for the starting server to go online
	for Stats.Status == "starting" {
		time.Sleep(1 * time.Second)
	}
	// if server is not online return
	if Stats.Status != "online" {
		return fmt.Errorf("StopMS: server is not online")
	}

	// player/StopMSRequests check
	if playersCheck {
		// check that there is only one StopMSRequest running and players <= 0,
		// if so proceed with server shutdown
		atomic.AddInt32(&Stats.StopMSRequests, -1)

		// check how many players are on the server
		playerCount, isFromServer := countPlayerSafe()
		logger.Logln(playerCount, "online players - number got from server:", isFromServer)
		if playerCount > 0 {
			return fmt.Errorf("StopMS: server is not empty")
		}

		// check if enough time has passed since last player disconnected

		if atomic.LoadInt32(&Stats.StopMSRequests) > 0 {
			return fmt.Errorf("StopMS: not enough time has passed since last player disconnected (StopMSRequests: %d)", Stats.StopMSRequests)
		}
	}

	// execute stop command
	_, errExec = Execute(config.ConfigRuntime.Commands.StopServer, "StopMS")
	if errExec != nil {
		return fmt.Errorf("StopMS: error executing minecraft server stop command: %v", errExec)
	}

	// if sigint is allowed, launch a function to check the shutdown of minecraft server
	if config.ConfigRuntime.Commands.StopServerAllowKill > 0 {
		go killMSifOnlineAfterTimeout()
	}

	return nil
}

// StopMSRequest increases StopMSRequests by one and starts the timer to execute StopMS(true) (with playersCheck)
// [goroutine]
func StopMSRequest() {
	atomic.AddInt32(&Stats.StopMSRequests, 1)

	// [goroutine]
	time.AfterFunc(
		time.Duration(config.ConfigRuntime.Msh.TimeBeforeStoppingEmptyServer)*time.Second,
		func() {
			err := StopMS(true)
			if err != nil {
				// avoid printing "server is not online" error since it can be very frequent
				// when updating the logging system this could be managed by logging it only at certain log levels
				if err.Error() != "StopMS: server is not online" {
					logger.Logln("StopMSRequest:", err)
				}
			}
		})
}

// killMSifOnlineAfterTimeout waits for the specified time and then if the server is still online, kills the server process
func killMSifOnlineAfterTimeout() {
	countdown := config.ConfigRuntime.Commands.StopServerAllowKill

	for countdown > 0 {
		// if server goes offline it's the correct behaviour -> return
		if Stats.Status == "offline" {
			return
		}

		countdown--
		time.Sleep(time.Second)
	}

	// save world before killing the server, do not check for errors
	logger.Logln("saving word before killing the minecraft server process")
	_, _ = Execute("save-all", "killMSifOnlineAfterTimeout")

	// give time to save word
	time.Sleep(10 * time.Second)

	// send kill signal to server
	logger.Logln("send kill signal to minecraft server process since it won't stop normally")
	err := ServTerm.cmd.Process.Kill()
	if err != nil {
		logger.Logln("killMSifOnlineAfterTimeout: %v", err)
	}
}
