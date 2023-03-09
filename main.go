package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"
)

import "strconv"

const NONE = 0

const (
	START = 1
	WAIT  = 2
	DONE  = 4
	JUST  = 8
)

const (
	SIMPLE  = 1
	TWO_WAY = 2
	LOOP    = 4
)

var started time.Time

func GetTicks() float32 {
	return float32(time.Since(started).Milliseconds())
}

func processClient(client *Client) string {
	buffer := make([]byte, 1024)
	mLen, err := client.conn.Read(buffer)

	if err != nil {
		fmt.Println("[SERVER] Error reading:", err.Error())

		client.conn.Close()
		client.conn = nil

		return ""
	}

	// fmt.Println("Received: ", string(buffer[:mLen]))
	// _, err = client.conn.Write([]byte("Thanks! Got your message:" + string(buffer[:mLen])))

	return string(buffer[:mLen])
}

type Ticks struct {
	frames, fps                                  uint32
	previous, current, delta, frame, accumulator float32
}

var ticks Ticks

type Timer struct {
	state, config                uint8
	delay, restartDelay, current float32
}

func (t *Timer) Set(state uint8) {
	t.state = state
	t.current = GetTicks()
}

func (t *Timer) Update() {
	diff := GetTicks() - t.current

	if t.state&JUST > 0 {
		t.state ^= JUST
	}

	if t.state == NONE {
		t.Set(START | JUST)
	} else if t.state&DONE > 0 {
		if t.config&LOOP > 0 {
			t.Set(START | JUST)
		}
	} else if diff >= t.delay {
		t.Set(DONE | JUST)
	}
}

func InitTicks(t *Ticks) {
	*t = Ticks{
		frames: 0, fps: 0,
		previous: 0, delta: 0.01, frame: 0, accumulator: 0,
		current: GetTicks(),
	}
}

func CreateTicks() Ticks {
	return Ticks{
		frames: 0, fps: 0,
		previous: 0, delta: 0.01, frame: 0, accumulator: 0,
		current: GetTicks(),
	}
}

func (t Ticks) dt() float32 {
	return t.delta
}

func (t *Ticks) Update() {
	t.frames++

	t.previous = t.current
	t.current = float32(GetTicks())
	t.frame = t.current - t.previous

	if t.frame > 0.25 {
		t.frame = 0.25
	}

	t.accumulator += t.frame

	t.fps = t.frames / (1 + uint32(GetTicks())/1000)
}

type Client struct {
	id, side, score int8
	conn            net.Conn
}

type Game struct {
	run              bool
	player1, player2 Client
	board            [9]*Client
	moves            int8
}

type Event struct {
	i      int
	client *Client
}

func CheckWinCondition(client *Client, board [9]*Client) *Client {
	for i := 0; i < 3; i++ {
		flag := true

		for j := 0; j < 3; j++ {
			if board[(3*i)+j] != client {
				flag = false
				break
			}
		}

		if flag {
			return client
		}

		flag = true

		for j := 0; j < 3; j++ {
			if board[(3*j)+i] != client {
				flag = false
				break
			}
		}

		if flag {
			return client
		}
	}

	if board[0] == client && board[4] == client && board[8] == client {
		return client
	}

	if board[2] == client && board[4] == client && board[6] == client {
		return client
	}

	return nil
}

func GameUpdate(game Game) {
	ticks := CreateTicks()

	game.player1.conn.Write([]byte("1"))
	game.player2.conn.Write([]byte("1"))

	turn, starting := false, false

	ch1, ch2 := make(chan Event), make(chan Event)

	var f = func(ch chan Event, client *Client) {
		for {
			string := processClient(client)

			if len(string) < 1 {
				break
			}

			i, err := strconv.Atoi(string)

			if err != nil {
				fmt.Println("couldn't read input")
			}

			ch <- Event{i: i, client: client}
		}
	}

	go f(ch1, &game.player1)
	go f(ch2, &game.player2)

	printTimer := Timer{
		config: SIMPLE | LOOP,
		delay:  1000,
		state:  NONE,
	}

	winTimer := Timer{
		config: SIMPLE,
		delay:  2000,
		state:  DONE,
	}

	for game.player1.conn != nil && game.player2.conn != nil {
		prev := time.Now()
		update := false

		winTimer.Update()

		if winTimer.state == DONE {
			select {
			case n := <-ch1:
				if turn == false && game.board[n.i] == nil {
					turn = true
					game.board[n.i] = &game.player1
					update = true
				}
				break
			case n := <-ch2:
				if turn == true && game.board[n.i] == nil {
					turn = false

					game.board[n.i] = &game.player2
					update = true
				}
				break
			default:

			}
		}

		if update || winTimer.state == DONE|JUST {
			result := ""
			for i := range game.board {
				if game.board[i] == &game.player1 {
					result += "0"
				} else if game.board[i] == &game.player2 {
					result += "1"
				} else {
					result += "-1"
				}
				result += ","
			}
			if CheckWinCondition(&game.player1, game.board) != nil {
				result += "0,"

				for i := 0; i < 9; i++ {
					game.board[i] = nil
				}

				if winTimer.state&DONE > 0 {
					game.player1.score++
					winTimer.Set(NONE)

					if starting {
						starting, turn = false, false
					} else {
						starting, turn = true, true
					}
				}

			} else if CheckWinCondition(&game.player2, game.board) != nil {
				result += "1,"

				for i := 0; i < 9; i++ {
					game.board[i] = nil
				}

				if winTimer.state&DONE > 0 {
					game.player2.score++
					winTimer.Set(NONE)

					if starting {
						starting, turn = false, false
					} else {
						starting, turn = true, true
					}
				}

			} else {
				result += "-1,"
			}

			score1 := strconv.Itoa(int(game.player1.score)) + "," +
				strconv.Itoa(int(game.player2.score)) + ","
			score2 := strconv.Itoa(int(game.player2.score)) + "," +
				strconv.Itoa(int(game.player1.score)) + ","
			game.player1.conn.Write([]byte(score1 + result))
			game.player2.conn.Write([]byte(score2 + result))
		}

		ticks.Update()
		printTimer.Update()
		curr := time.Since(prev).Milliseconds()

		if curr < 32 {
			time.Sleep(time.Millisecond * time.Duration(32-curr))
		}
	}

	if game.player1.conn != nil {
		game.player1.conn.Close()
	}

	if game.player2.conn != nil {
		game.player2.conn.Close()
	}

	fmt.Println("game ended for some reason foo")
}

type Connection struct {
	protocol, host, port string
}

func ReadConfig() Connection {
	file, err := os.Open("config")

	if err != nil {
		fmt.Println(err)
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	scanner.Scan()
	connection := Connection{protocol: scanner.Text()}

	scanner.Scan()
	connection.host = scanner.Text()

	scanner.Scan()
	connection.port = scanner.Text()

	if err := scanner.Err(); err != nil {
		fmt.Println(err)
	}

	return connection
}

func main() {
	config := ReadConfig()

	InitTicks(&ticks)

	fmt.Println("Server Running...")

	started = time.Now()

	server, err := net.Listen(config.protocol, config.host+":"+config.port)

	if err != nil {
		fmt.Println("Error listening:", err.Error())

		os.Exit(1)
	}

	defer server.Close()

	fmt.Println("Listening on " + config.host + ":" + config.port)

	currId := int8(0)

	for {
		conn1, err := server.Accept()

		if err != nil {
			fmt.Println("Error accepting 1: ", err.Error())
			os.Exit(1)
		}

		fmt.Println("client 1 connected")
		_, err = conn1.Write([]byte("0"))

		conn2, err := server.Accept()

		if err != nil {
			fmt.Println("Error accepting 2: ", err.Error())
			os.Exit(1)
		}

		fmt.Println("client 2 connected")

		_, err = conn2.Write([]byte("1"))

		go GameUpdate(Game{run: true,
			board: [9]*Client{nil, nil, nil, nil, nil, nil, nil, nil, nil},
			moves: 0,
			player1: Client{
				id:    currId,
				conn:  conn1,
				side:  0,
				score: 0,
			},
			player2: Client{
				id:    currId + 1,
				conn:  conn2,
				side:  1,
				score: 0,
			},
		})

		currId += 2

		fmt.Println("waiting for next pair")
	}

	fmt.Println("server terminated")
}
