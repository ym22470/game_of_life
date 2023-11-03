package gol

import (
	"fmt"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

func copySlice(src [][]byte) [][]byte {
	dst := make([][]byte, len(src))
	for i := range src {
		dst[i] = make([]byte, len(src[i]))
		copy(dst[i], src[i])
	}
	return dst
}

// distributor divides the work between workers and interacts with other goroutines.i.e no shared memory!
func distributor(p Params, c distributorChannels) {
	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%dx%d", p.ImageHeight, p.ImageWidth)

	// TODO: Create a 2D slice to store the world.
	rows := p.ImageHeight
	cols := p.ImageWidth
	world := make([][]byte, rows)
	for i := range world {
		world[i] = make([]byte, cols)
	}
	//read the input from io
	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			world[i][j] = <-c.ioInput
		}
	}
	turn := 0
	// TODO: Execute all turns of the Game of Life.
	for ; turn < p.Turns; turn++ {
		if p.Threads == 1 {
			world = calculateNextState(p, world, 0, p.ImageHeight)
		} else {
			chans := make([]chan [][]byte, p.Threads)
			for i := 0; i < p.Threads; i++ {
				chans[i] = make(chan [][]byte)
				a := i * (p.ImageHeight / p.Threads)
				b := (i + 1) * (p.ImageHeight / p.Threads)
				if i == p.Threads-1 {
					b = p.ImageHeight
				}
				worldCopy := copySlice(world) //to handle data race condition by passing a copy of world to goroutines
				go worker(p, worldCopy, a, b, chans[i])
			}
			//combine all the strips produced by workers
			for i := 0; i < p.Threads; i++ {
				strip := <-chans[i]
				startRow := i * (p.ImageHeight / p.Threads)
				for r, row := range strip {
					world[startRow+r] = row
				}
			}
		}
		//report alive cells every 2 seconds
		time.Sleep(2 * time.Second)
		c.events <- AliveCellsCount{CellsCount: len(calculateAliveCells(p, world)), CompletedTurns: turn + 1}
		c.events <- TurnComplete{CompletedTurns: turn + 1}
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
	var turnState FinalTurnComplete // instantiate the new struct
	turnState.CompletedTurns = turn
	turnState.Alive = calculateAliveCells(p, world)
	c.events <- turnState // sending event to testing framework via channel
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.events <- StateChange{turn, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func worker(p Params, world [][]byte, startY, endY int, out chan<- [][]byte) {
	results := calculateNextState(p, world, startY, endY)
	out <- results
	close(out)
}

func calculateNextState(p Params, world [][]byte, start, end int) [][]byte {
	//initialise variables
	rows := len(world)
	if rows == 0 {
		return world
	}
	cols := len(world[0])

	// Create a copy to hold the changes
	newWorld := make([][]byte, end-start)
	for i := range newWorld {
		newWorld[i] = make([]byte, cols)
		copy(newWorld[i], world[i])
	}

	for i := start; i < end; i++ {
		for j := range world[i] {
			count := 0
			// Iterate over the 8 neighbors
			//3 offsets
			for x := -1; x <= 1; x++ {
				for y := -1; y <= 1; y++ {
					// Skip the center cell
					if x == 0 && y == 0 {
						continue
					}
					ni, nj := i+x, j+y
					// Handle boundaries
					if ni < 0 {
						ni = rows - 1
					} else if ni >= rows {
						ni = 0
					}
					if nj < 0 {
						nj = cols - 1
					} else if nj >= cols {
						nj = 0
					}
					// Check cell value
					if world[ni][nj] == 255 {
						//data race,but no write operation??
						count++
					}
				}
			}
			if world[i][j] == 255 {
				if count > 3 || count < 2 {
					newWorld[i-start][j] = 0
				} else {
					newWorld[i-start][j] = 255
				}
			} else if world[i][j] == 0 {
				if count == 3 {
					newWorld[i-start][j] = 255
				} else {
					newWorld[i-start][j] = 0
				}
			}
		}
	}
	return newWorld
}

func calculateAliveCells(p Params, world [][]byte) []util.Cell {
	var cs []util.Cell
	for i, _ := range world {
		for i2, v := range world[i] {
			if v == 255 {
				c := util.Cell{
					X: i2,
					Y: i,
				}
				cs = append(cs, c)
			}
		}
	}
	return cs
}
