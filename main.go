package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ---------- Глобальные переменные (ДО ВСЕХ ФУНКЦИЙ) ----------
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ---------- Конфигурационные структуры ----------
type GameConfig struct {
	WorldSize              int `json:"world_size"`
	TickIntervalSeconds    int `json:"tick_interval_seconds"`
	RespawnIntervalSeconds int `json:"respawn_interval_seconds"`
	MaxCellFill            int `json:"max_cell_fill"`
	WildnessThreshold      int `json:"wildness_threshold"`
	HourMinutesReal        int `json:"hour_minutes_real"`

	Hunger struct {
		DecayPerHour       int `json:"decay_per_hour"`
		StartValue         int `json:"start_value"`
		CriticalThreshold  int `json:"critical_threshold"`
		DeathMinutesAtZero int `json:"death_minutes_at_zero"`
	} `json:"hunger"`

	Thirst struct {
		DecayPerHour       int `json:"decay_per_hour"`
		StartValue         int `json:"start_value"`
		CriticalThreshold  int `json:"critical_threshold"`
		DeathMinutesAtZero int `json:"death_minutes_at_zero"`
	} `json:"thirst"`
}

type ResourceConfig struct {
	Name               string `json:"name"`
	BaseHarvestAmount  int    `json:"base_harvest_amount"`
	HarvestTimeSeconds int    `json:"harvest_time_seconds"`
	FillCostPerUnit    int    `json:"fill_cost_per_unit"`
	RespawnRateForest  int    `json:"respawn_rate_forest"`
	RespawnRatePlain   int    `json:"respawn_rate_plain"`
	RespawnRateWater   int    `json:"respawn_rate_water"`
	MaxForest          int    `json:"max_forest"`
	MaxPlain           int    `json:"max_plain"`
	MaxWater           int    `json:"max_water"`
	FoodValue          int    `json:"food_value"`
	WaterValue         int    `json:"water_value"`
}

type BuildingConfig struct {
	Name              string         `json:"name"`
	WoodCost          int            `json:"wood_cost"`
	StoneCost         int            `json:"stone_cost"`
	FillCost          int            `json:"fill_cost"`
	BuildTimeSeconds  int            `json:"build_time_seconds"`
	Effects           map[string]int `json:"effects"`
	RequiredResources []string       `json:"required_resources"`
}

type PlayerConfig struct {
	StartPosition struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"start_position"`
	StartInventory map[string]int `json:"start_inventory"`
	StartStats     struct {
		Hunger    int `json:"hunger"`
		Thirst    int `json:"thirst"`
		Energy    int `json:"energy"`
		Attention int `json:"attention"`
	} `json:"start_stats"`
	MaxStats struct {
		Hunger    int `json:"hunger"`
		Thirst    int `json:"thirst"`
		Energy    int `json:"energy"`
		Attention int `json:"attention"`
	} `json:"max_stats"`
	CarryCapacity    int     `json:"carry_capacity"`
	BaseHarvestSpeed float64 `json:"base_harvest_speed"`
}

// ---------- Глобальные конфиги ----------
var (
	gameConfig      *GameConfig
	resourcesConfig map[string]*ResourceConfig
	buildingsConfig map[string]*BuildingConfig
	playerConfig    *PlayerConfig
)

// ---------- Типы ----------
type Coord struct {
	X, Y int
}

type Building struct {
	Type    string    `json:"type"`
	X       int       `json:"x"`
	Y       int       `json:"y"`
	Owner   string    `json:"owner"`
	BuiltAt time.Time `json:"builtAt"`
}

type Cell struct {
	TerrainType string               `json:"terrainType"`
	Fill        int                  `json:"fill"`
	WoodLeft    int                  `json:"woodLeft"`
	StoneLeft   int                  `json:"stoneLeft"`
	BerryLeft   int                  `json:"berryLeft"`
	WaterLeft   int                  `json:"waterLeft"`
	Players     []string             `json:"players"`
	Buildings   map[string]*Building `json:"buildings"`
}

type Player struct {
	ID        string         `json:"id"`
	X         int            `json:"x"`
	Y         int            `json:"y"`
	Inventory map[string]int `json:"inventory"`
	Stats     struct {
		Hunger    int `json:"hunger"`
		Thirst    int `json:"thirst"`
		Energy    int `json:"energy"`
		Attention int `json:"attention"`
	} `json:"stats"`
	Buffs      map[string]int `json:"buffs"`
	Conn       *websocket.Conn
	send       chan []byte
	harvesting *HarvestTask
}

type HarvestTask struct {
	Resource  string
	StartTime time.Time
	Duration  time.Duration
	Cancel    chan bool
}

type World struct {
	mu      sync.RWMutex
	cells   map[Coord]*Cell
	players map[*Player]bool
}

type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type MovePayload struct {
	Direction string `json:"direction"`
}

type HarvestPayload struct {
	Resource string `json:"resource"`
}

type BuildPayload struct {
	Building string `json:"building"`
}

type EatPayload struct {
	Resource string `json:"resource"`
}

type DrinkPayload struct {
	Resource string `json:"resource"`
}

// ---------- Загрузка конфигурации ----------
func loadGameConfig() error {
	data, err := os.ReadFile("config/game.json")
	if err != nil {
		return err
	}
	gameConfig = &GameConfig{}
	return json.Unmarshal(data, gameConfig)
}

func loadResourcesConfig() error {
	data, err := os.ReadFile("config/resources.json")
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &resourcesConfig)
}

func loadBuildingsConfig() error {
	data, err := os.ReadFile("config/buildings.json")
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &buildingsConfig)
}

func loadPlayerConfig() error {
	data, err := os.ReadFile("config/player.json")
	if err != nil {
		return err
	}
	playerConfig = &PlayerConfig{}
	return json.Unmarshal(data, playerConfig)
}

// ---------- Инициализация мира ----------
func NewWorld() *World {
	world := &World{
		cells:   make(map[Coord]*Cell),
		players: make(map[*Player]bool),
	}

	rand.Seed(time.Now().UnixNano())

	for x := 0; x < gameConfig.WorldSize; x++ {
		for y := 0; y < gameConfig.WorldSize; y++ {
			terrain := "plain"
			woodLeft := 0
			stoneLeft := 0
			berryLeft := 0
			waterLeft := 0

			if x == 0 || x == gameConfig.WorldSize-1 || y == 0 || y == gameConfig.WorldSize-1 {
				terrain = "forest"
				woodLeft = 50 + rand.Intn(50)
				stoneLeft = 10 + rand.Intn(20)
				berryLeft = 20 + rand.Intn(30)
			} else if x == 5 && y == 5 {
				terrain = "forest"
				woodLeft = 100 + rand.Intn(50)
				stoneLeft = 30 + rand.Intn(20)
				berryLeft = 40 + rand.Intn(30)
			} else if x > 2 && x < 7 && y > 2 && y < 7 {
				terrain = "plain"
				woodLeft = 10 + rand.Intn(20)
				stoneLeft = 5 + rand.Intn(10)
				berryLeft = 15 + rand.Intn(20)
			}

			if (x == 0 && y == 0) || (x == gameConfig.WorldSize-1 && y == gameConfig.WorldSize-1) {
				terrain = "water"
				waterLeft = 100
			}

			world.cells[Coord{x, y}] = &Cell{
				TerrainType: terrain,
				Fill:        0,
				WoodLeft:    woodLeft,
				StoneLeft:   stoneLeft,
				BerryLeft:   berryLeft,
				WaterLeft:   waterLeft,
				Players:     []string{},
				Buildings:   make(map[string]*Building),
			}
		}
	}

	return world
}

// ---------- Респавн ресурсов ----------
func (world *World) RespawnResources() {
	world.mu.Lock()
	defer world.mu.Unlock()

	for _, cell := range world.cells {
		if cell.TerrainType == "water" {
			if res, ok := resourcesConfig["water"]; ok {
				if cell.WaterLeft < res.MaxWater && cell.Fill < gameConfig.WildnessThreshold {
					cell.WaterLeft += res.RespawnRateWater
					if cell.WaterLeft > res.MaxWater {
						cell.WaterLeft = res.MaxWater
					}
				}
			}
			continue
		}

		if cell.Fill >= gameConfig.WildnessThreshold {
			continue
		}

		// Респавн дерева
		if woodRes, ok := resourcesConfig["wood"]; ok {
			if cell.TerrainType == "forest" && cell.WoodLeft < woodRes.MaxForest {
				cell.WoodLeft += woodRes.RespawnRateForest
				if cell.WoodLeft > woodRes.MaxForest {
					cell.WoodLeft = woodRes.MaxForest
				}
			} else if cell.TerrainType == "plain" && cell.WoodLeft < woodRes.MaxPlain {
				cell.WoodLeft += woodRes.RespawnRatePlain
				if cell.WoodLeft > woodRes.MaxPlain {
					cell.WoodLeft = woodRes.MaxPlain
				}
			}
		}

		// Респавн камня
		if stoneRes, ok := resourcesConfig["stone"]; ok {
			if cell.StoneLeft < stoneRes.MaxPlain {
				cell.StoneLeft += stoneRes.RespawnRatePlain
				if cell.StoneLeft > stoneRes.MaxPlain {
					cell.StoneLeft = stoneRes.MaxPlain
				}
			}
		}

		// Респавн ягод
		if berryRes, ok := resourcesConfig["berry"]; ok {
			maxBerry := berryRes.MaxPlain
			if cell.TerrainType == "forest" {
				maxBerry = berryRes.MaxForest
			}
			if cell.BerryLeft < maxBerry {
				rate := berryRes.RespawnRatePlain
				if cell.TerrainType == "forest" {
					rate = berryRes.RespawnRateForest
				}
				cell.BerryLeft += rate
				if cell.BerryLeft > maxBerry {
					cell.BerryLeft = maxBerry
				}
			}
		}
	}
}

// ---------- Добавление игрока ----------
func (world *World) AddPlayer(conn *websocket.Conn) *Player {
	world.mu.Lock()
	defer world.mu.Unlock()

	player := &Player{
		ID:        generatePlayerID(),
		X:         playerConfig.StartPosition.X,
		Y:         playerConfig.StartPosition.Y,
		Inventory: make(map[string]int),
		Buffs:     make(map[string]int),
		Conn:      conn,
		send:      make(chan []byte, 256),
	}

	// Копируем стартовый инвентарь
	for k, v := range playerConfig.StartInventory {
		player.Inventory[k] = v
	}

	// Устанавливаем стартовые статы
	player.Stats.Hunger = playerConfig.StartStats.Hunger
	player.Stats.Thirst = playerConfig.StartStats.Thirst
	player.Stats.Energy = playerConfig.StartStats.Energy
	player.Stats.Attention = playerConfig.StartStats.Attention

	world.players[player] = true
	world.cells[Coord{player.X, player.Y}].Players = append(world.cells[Coord{player.X, player.Y}].Players, player.ID)

	go player.writePump()
	go player.statsTicker()

	return player
}

// ---------- Таймер статов игрока ----------
func (p *Player) statsTicker() {
	ticker := time.NewTicker(time.Duration(gameConfig.HourMinutesReal) * time.Minute / 60)
	defer ticker.Stop()

	for range ticker.C {
		p.updateStats()
	}
}

func (p *Player) updateStats() {
	p.Stats.Hunger -= gameConfig.Hunger.DecayPerHour
	p.Stats.Thirst -= gameConfig.Thirst.DecayPerHour

	if p.Stats.Hunger < 0 {
		p.Stats.Hunger = 0
	}
	if p.Stats.Thirst < 0 {
		p.Stats.Thirst = 0
	}

	// Критический уровень
	if p.Stats.Hunger < gameConfig.Hunger.CriticalThreshold {
		p.Stats.Attention -= 5
		p.Stats.Energy -= 5
	}
	if p.Stats.Thirst < gameConfig.Thirst.CriticalThreshold {
		p.Stats.Attention -= 10
		p.Stats.Energy -= 10
	}
}

// ---------- Еда и питьё ----------
func (p *Player) Eat(resource string) bool {
	resConfig, ok := resourcesConfig[resource]
	if !ok || resConfig.FoodValue == 0 {
		return false
	}

	if p.Inventory[resource] <= 0 {
		return false
	}

	p.Inventory[resource]--
	p.Stats.Hunger += resConfig.FoodValue
	if p.Stats.Hunger > playerConfig.MaxStats.Hunger {
		p.Stats.Hunger = playerConfig.MaxStats.Hunger
	}

	return true
}

func (p *Player) Drink(resource string) bool {
	resConfig, ok := resourcesConfig[resource]
	if !ok || resConfig.WaterValue == 0 {
		return false
	}

	if p.Inventory[resource] <= 0 {
		return false
	}

	p.Inventory[resource]--
	p.Stats.Thirst += resConfig.WaterValue
	if p.Stats.Thirst > playerConfig.MaxStats.Thirst {
		p.Stats.Thirst = playerConfig.MaxStats.Thirst
	}

	return true
}

// ---------- Удаление игрока ----------
func (world *World) RemovePlayer(p *Player) {
	world.mu.Lock()
	defer world.mu.Unlock()

	if p.harvesting != nil {
		close(p.harvesting.Cancel)
		p.harvesting = nil
	}

	if cell, ok := world.cells[Coord{p.X, p.Y}]; ok {
		newPlayers := []string{}
		for _, id := range cell.Players {
			if id != p.ID {
				newPlayers = append(newPlayers, id)
			}
		}
		cell.Players = newPlayers
	}

	delete(world.players, p)
	close(p.send)
}

// ---------- Обработка движений ----------
func (world *World) MovePlayer(p *Player, direction string) bool {
	world.mu.Lock()
	defer world.mu.Unlock()

	// Отменяем текущую добычу
	if p.harvesting != nil {
		close(p.harvesting.Cancel)
		p.harvesting = nil
	}

	newX, newY := p.X, p.Y
	switch direction {
	case "up":
		newY--
	case "down":
		newY++
	case "left":
		newX--
	case "right":
		newX++
	default:
		return false
	}

	if newX < 0 || newX >= gameConfig.WorldSize || newY < 0 || newY >= gameConfig.WorldSize {
		return false
	}

	newCell, ok := world.cells[Coord{newX, newY}]
	if !ok || newCell.TerrainType == "water" {
		return false
	}

	oldCell := world.cells[Coord{p.X, p.Y}]
	newPlayersList := []string{}
	for _, id := range oldCell.Players {
		if id != p.ID {
			newPlayersList = append(newPlayersList, id)
		}
	}
	oldCell.Players = newPlayersList

	p.X, p.Y = newX, newY
	newCell.Players = append(newCell.Players, p.ID)

	return true
}

// ---------- Обработка сбора ресурсов (асинхронный) ----------
func (world *World) StartHarvest(p *Player, resource string) bool {
	world.mu.Lock()
	defer world.mu.Unlock()

	cell, ok := world.cells[Coord{p.X, p.Y}]
	if !ok {
		return false
	}

	resConfig, ok := resourcesConfig[resource]
	if !ok {
		return false
	}

	// Проверяем наличие ресурса
	var available int
	switch resource {
	case "wood":
		available = cell.WoodLeft
	case "stone":
		available = cell.StoneLeft
	case "berry":
		available = cell.BerryLeft
	case "water":
		if cell.TerrainType != "water" {
			return false
		}
		available = cell.WaterLeft
	default:
		return false
	}

	if available <= 0 {
		return false
	}

	// Если уже добывает, отменяем
	if p.harvesting != nil {
		close(p.harvesting.Cancel)
		p.harvesting = nil
	}

	// Запускаем добычу в горутине
	cancel := make(chan bool)
	p.harvesting = &HarvestTask{
		Resource:  resource,
		StartTime: time.Now(),
		Duration:  time.Duration(resConfig.HarvestTimeSeconds) * time.Second,
		Cancel:    cancel,
	}

	go func() {
		select {
		case <-time.After(p.harvesting.Duration):
			world.CompleteHarvest(p, resource)
		case <-cancel:
			world.mu.Lock()
			p.harvesting = nil
			world.mu.Unlock()
		}
	}()

	return true
}

func (world *World) CompleteHarvest(p *Player, resource string) {
	world.mu.Lock()
	defer world.mu.Unlock()

	if p.harvesting == nil || p.harvesting.Resource != resource {
		return
	}

	cell, ok := world.cells[Coord{p.X, p.Y}]
	if !ok {
		p.harvesting = nil
		return
	}

	resConfig := resourcesConfig[resource]
	amount := resConfig.BaseHarvestAmount

	switch resource {
	case "wood":
		if cell.WoodLeft < amount {
			amount = cell.WoodLeft
		}
		cell.WoodLeft -= amount
		p.Inventory["wood"] += amount
		cell.Fill += amount * resConfig.FillCostPerUnit

	case "stone":
		if cell.StoneLeft < amount {
			amount = cell.StoneLeft
		}
		cell.StoneLeft -= amount
		p.Inventory["stone"] += amount
		cell.Fill += amount * resConfig.FillCostPerUnit

	case "berry":
		if cell.BerryLeft < amount {
			amount = cell.BerryLeft
		}
		cell.BerryLeft -= amount
		p.Inventory["berry"] += amount
		cell.Fill += amount * resConfig.FillCostPerUnit

	case "water":
		if cell.WaterLeft < amount {
			amount = cell.WaterLeft
		}
		cell.WaterLeft -= amount
		p.Inventory["water"] += amount
	}

	if cell.Fill > gameConfig.MaxCellFill {
		cell.Fill = gameConfig.MaxCellFill
	}

	p.harvesting = nil
}

// ---------- Постройка ----------
func (world *World) Build(p *Player, buildingType string) bool {
	world.mu.Lock()
	defer world.mu.Unlock()

	buildConfig, ok := buildingsConfig[buildingType]
	if !ok {
		return false
	}

	cell, ok := world.cells[Coord{p.X, p.Y}]
	if !ok {
		return false
	}

	// Проверяем, не построено ли уже
	if _, exists := cell.Buildings[buildingType]; exists {
		return false
	}

	// Проверяем ресурсы
	if p.Inventory["wood"] < buildConfig.WoodCost {
		return false
	}
	if p.Inventory["stone"] < buildConfig.StoneCost {
		return false
	}

	// Проверяем место
	if cell.Fill+buildConfig.FillCost > gameConfig.MaxCellFill {
		return false
	}

	// Строим
	p.Inventory["wood"] -= buildConfig.WoodCost
	p.Inventory["stone"] -= buildConfig.StoneCost
	cell.Fill += buildConfig.FillCost
	cell.Buildings[buildingType] = &Building{
		Type:    buildingType,
		X:       p.X,
		Y:       p.Y,
		Owner:   p.ID,
		BuiltAt: time.Now(),
	}

	// Применяем эффекты
	for effect, value := range buildConfig.Effects {
		p.Buffs[effect] = value
	}

	return true
}

// ---------- Получение текущего состояния мира ----------
func (world *World) GetState() map[string]interface{} {
	world.mu.RLock()
	defer world.mu.RUnlock()

	cellsMap := make(map[string]*Cell)
	for coord, cell := range world.cells {
		key := string(rune(coord.X)) + "," + string(rune(coord.Y))
		cellsMap[key] = cell
	}

	playersMap := make(map[string]*Player)
	for p := range world.players {
		playersMap[p.ID] = p
	}

	return map[string]interface{}{
		"cells":   cellsMap,
		"players": playersMap,
	}
}

// ---------- Рассылка обновлений ----------
func (world *World) BroadcastState() {
	world.mu.RLock()
	defer world.mu.RUnlock()

	state := world.GetState()
	data, err := json.Marshal(state)
	if err != nil {
		log.Print("Marshal error:", err)
		return
	}

	for p := range world.players {
		select {
		case p.send <- data:
		default:
		}
	}
}

// ---------- Helper функции ----------
func generatePlayerID() string {
	return "player_" + time.Now().Format("20060102150405") + "_" + randomString(4)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}

// ---------- WebSocket обработчик ----------
func (world *World) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("Upgrade error:", err)
		return
	}
	defer conn.Close()

	player := world.AddPlayer(conn)
	defer world.RemovePlayer(player)

	initialState := world.GetState()
	initialState["currentPlayerId"] = player.ID
	data, _ := json.Marshal(initialState)
	player.send <- data

	for {
		_, msgData, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg Message
		if err := json.Unmarshal(msgData, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "move":
			var payload MovePayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				continue
			}
			world.MovePlayer(player, payload.Direction)

		case "harvest":
			var payload HarvestPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				continue
			}
			world.StartHarvest(player, payload.Resource)

		case "build":
			var payload BuildPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				continue
			}
			world.Build(player, payload.Building)

		case "eat":
			var payload EatPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				continue
			}
			player.Eat(payload.Resource)

		case "drink":
			var payload DrinkPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				continue
			}
			player.Drink(payload.Resource)
		}

		world.BroadcastState()
	}
}

// writePump отправляет сообщения клиенту
func (p *Player) writePump() {
	for msg := range p.send {
		if err := p.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}

// ---------- Главная функция ----------
func main() {
	// Загружаем конфигурацию
	if err := loadGameConfig(); err != nil {
		log.Fatal("Ошибка загрузки game.json:", err)
	}
	if err := loadResourcesConfig(); err != nil {
		log.Fatal("Ошибка загрузки resources.json:", err)
	}
	if err := loadBuildingsConfig(); err != nil {
		log.Fatal("Ошибка загрузки buildings.json:", err)
	}
	if err := loadPlayerConfig(); err != nil {
		log.Fatal("Ошибка загрузки player.json:", err)
	}

	world := NewWorld()

	// Запускаем респавн ресурсов
	go func() {
		ticker := time.NewTicker(time.Duration(gameConfig.RespawnIntervalSeconds) * time.Second)
		for range ticker.C {
			world.RespawnResources()
			world.BroadcastState()
		}
	}()

	// Периодическая рассылка состояния
	go func() {
		ticker := time.NewTicker(time.Duration(gameConfig.TickIntervalSeconds) * time.Second)
		for range ticker.C {
			world.BroadcastState()
		}
	}()

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/ws", world.HandleWebSocket)

	log.Println("=== ЭТАП 3 ===")
	log.Println("Сервер запущен на http://localhost:8080")
	log.Println("Новое: конфигурация из JSON файлов")
	log.Println("Новое: голод и жажда (медленное снижение)")
	log.Println("Новое: еда (ягоды) и вода")
	log.Println("Новое: асинхронная добыча ресурсов")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
