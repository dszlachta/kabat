package report

import "log"

var notify func(error)

// SetNotifier registers the GUI error presenter (Wails dialog). Native-host
// process leaves this nil so only the log runs.
func SetNotifier(fn func(error)) {
	notify = fn
}

// Error logs err and, when a notifier is set, shows it to the user.
func Error(err error) {
	if err == nil {
		return
	}
	log.Print(err)
	if notify != nil {
		notify(err)
	}
}
