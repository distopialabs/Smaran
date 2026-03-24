package main

import (
	"os"
	"runtime/pprof"
)

// ProfileCPU starts CPU profiling and returns a function to stop profiling and clean up.
// Call the returned function with defer in main, e.g. defer ProfileCPU(path)()
func ProfileCPU(profilePath string) func() {
	// create profilePath if it doesn't exist
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		err = os.MkdirAll(profilePath, 0755)
		if err != nil {
			panic(err)

		}
	}
	f, err := os.Create(profilePath + "/cpu.prof")
	if err != nil {
		panic(err)
	}

	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		panic(err)
	}

	return func() {
		pprof.StopCPUProfile()
		f.Close()
	}
}
