package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ---------- Модели данных ----------
type Tile struct {
	Surface    string  `json:"surface"`
	Path       string  `json:"path"`
	TrailCount int     `json:"-"`
	Bridge     *Bridge `json:"bridge,omitempty"`
}

type Bridge struct {
	Material string `json:"material"`
	HP       int    `json:"hp"`
}

type GameObject struct {
	X          int            `json:"x"`
	Layer      string         `json:"layer"`
	Type       string         `json:"type"`
	State      string         `json:"state"`
	StageIndex int            `json:"stageIndex"`
	StageTimer int64          `json:"stageTimer"`
	Resources  map[string]int `json:"resources,omitempty"`
	OwnerHouse string         `json:"ownerHouse,omitempty"`
	HP         int            `json:"hp,omitempty"`
}

type Animal struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"`
	X            float64 `json:"x"`
	HP           int     `json:"hp"`
	MaxHP        int     `json:"maxHp"`
	State        string  `json:"state"`
	CorpseTimer  int64   `json:"corpseTimer,omitempty"`
	TargetX      float64 `json:"targetX,omitempty"`
	TargetPlayer string  `json:"targetPlayer,omitempty"`
	MoveDir      int     `json:"moveDir"`
	Speed        float64 `json:"speed"`
	WanderTarget float64 `json:"wanderTarget"`
	WanderTimer  int64   `json:"wanderTimer"`
	SleepTimer   int64   `json:"sleepTimer"`
	EatTargetX   int     `json:"eatTargetX"`
	EatTimer     int64   `json:"eatTimer"`
}

type AnimalConfig struct {
	ID             string   `json:"id"`
	NameKey        string   `json:"name_key"`
	HP             int      `json:"hp"`
	SpeedWalk      float64  `json:"speed_walk"`
	SpeedRun       float64  `json:"speed_run"`
	Damage         int      `json:"damage"`
	PreferredFood  []string `json:"preferred_food"`
	Aggressive     bool     `json:"aggressive"`
	FleeDistance   float64  `json:"flee_distance"`
	AttackDistance float64  `json:"attack_distance"`
	SleepChance    float64  `json:"sleep_chance"`
}

type Player struct {
	ID           string
	Name         string
	HouseName    string
	X            float64
	Direction    int
	Running      bool
	Inventory    map[string]int
	Hunger       int
	Thirst       int
	State        string
	Relationship map[string]string
	Language     string
	conn         *websocket.Conn
	send         chan []byte
}

type Location struct {
	Name             string                   `json:"name"`
	Biome            string                   `json:"biome"`
	Width            int                      `json:"width"`
	Ground           GroundConfig             `json:"ground"`
	Decors           []map[string]interface{} `json:"decors"`
	NextLocation     string                   `json:"next_location,omitempty"`
	PreviousLocation string                   `json:"previous_location,omitempty"`
	TransitionX      int                      `json:"transition_x,omitempty"`
	DayNightCycle    int                      `json:"day_night_cycle_seconds"`
	SpawnSchedule    string                   `json:"spawn_schedule"`
	MaxBoars         int                      `json:"max_boars"`
	MaxHares         int                      `json:"max_hares"`
	Tiles            []Tile
	FrontObjects     []GameObject
	BackObjects      []GameObject
	Animals          []Animal
	RespawnQueue     []RespawnItem
	mu               sync.RWMutex
}

type GroundConfig struct {
	DefaultTile string         `json:"default_tile"`
	Tiles       map[int]string `json:"tiles"`
}

type WorldState struct {
	Players      map[string]*Player
	Location     *Location
	GameTime     float64
	TimeOfDay    string
	Paused       bool
	LastSaveTime time.Time
	mu           sync.RWMutex
}

type RespawnItem struct {
	X         int
	Layer     string
	Type      string
	StageIdx  int
	RespawnAt time.Time
}

type ObjectStage struct {
	NameKey         string   `json:"name_key"`
	DurationSeconds int      `json:"duration_seconds"`
	Interactions    []string `json:"interactions"`
}

type ObjectType struct {
	ID       string        `json:"id"`
	Category string        `json:"category"`
	Stages   []ObjectStage `json:"stages"`
}

type Item struct {
	ID         string  `json:"id"`
	Stackable  bool    `json:"stackable"`
	MaxStack   int     `json:"max_stack"`
	Weight     float64 `json:"weight"`
	FoodValue  int     `json:"food_value"`
	WaterValue int     `json:"water_value"`
	Poisonous  bool    `json:"poisonous"`
}

type Interaction struct {
	ID           string `json:"id"`
	ResultItem   string `json:"result_item"`
	AmountMin    int    `json:"amount_min"`
	AmountMax    int    `json:"amount_max"`
	ToolRequired string `json:"tool_required"`
	Skill        string `json:"skill"`
	XP           int    `json:"xp"`
}

type LocationConfig struct {
	NameKey           string                   `json:"name_key"`
	Biome             string                   `json:"biome"`
	Width             int                      `json:"width"`
	Ground            GroundConfig             `json:"ground"`
	Objects           []LocationObjectConfig   `json:"objects"`
	Decors            []map[string]interface{} `json:"decors"`
	NextLocation      string                   `json:"next_location"`
	PreviousLocation  string                   `json:"previous_location"`
	TransitionX       int                      `json:"transition_x"`
	DayNightCycleSecs int                      `json:"day_night_cycle_seconds"`
	SpawnSchedule     string                   `json:"spawn_schedule"`
	MaxBoars          int                      `json:"max_boars"`
	MaxHares          int                      `json:"max_hares"`
}

type LocationObjectConfig struct {
	X          int    `json:"x"`
	Layer      string `json:"layer"`
	TypeID     string `json:"type_id"`
	Stage      int    `json:"stage"`
	StageTimer int64  `json:"stage_timer"`
	OwnerHouse string `json:"owner_house,omitempty"`
}

type SaveData struct {
	Locations      map[string]*Location `json:"locations"`
	Players        map[string]*Player   `json:"players"`
	GameTime       float64              `json:"gameTime"`
	CurrentLocName string               `json:"currentLocName"`
	SavedAt        time.Time            `json:"savedAt"`
}

var objectTypes map[string]ObjectType
var animalConfigs map[string]AnimalConfig
var items map[string]Item
var interactions map[string]Interaction
var translations map[string]map[string]string
var locations map[string]*Location
var currentLocation *Location
var world *WorldState
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ---------- Загрузка конфигов ----------
func loadObjectTypes(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var config struct {
		ObjectTypes []ObjectType   `json:"object_types"`
		Animals     []AnimalConfig `json:"animals"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	objectTypes = make(map[string]ObjectType)
	for _, ot := range config.ObjectTypes {
		objectTypes[ot.ID] = ot
	}
	animalConfigs = make(map[string]AnimalConfig)
	for _, ac := range config.Animals {
		animalConfigs[ac.ID] = ac
	}
	return nil
}

func loadItems(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var config struct {
		Items []Item `json:"items"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	items = make(map[string]Item)
	for _, it := range config.Items {
		items[it.ID] = it
	}
	return nil
}

func loadInteractions(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var config struct {
		Interactions []Interaction `json:"interactions"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	interactions = make(map[string]Interaction)
	for _, in := range config.Interactions {
		interactions[in.ID] = in
	}
	return nil
}

func loadTranslations(langDir string) error {
	translations = make(map[string]map[string]string)
	files, err := os.ReadDir(langDir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if !file.IsDir() && (file.Name() == "ru.json" || file.Name() == "en.json") {
			data, err := os.ReadFile(langDir + "/" + file.Name())
			if err != nil {
				continue
			}
			var trans map[string]string
			if err := json.Unmarshal(data, &trans); err != nil {
				continue
			}
			if lang, ok := trans["language"]; ok {
				translations[lang] = trans
			}
		}
	}
	return nil
}

func loadAllLocations(locDir string) error {
	locations = make(map[string]*Location)
	files, err := os.ReadDir(locDir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if !file.IsDir() && (file.Name() == "weeping_spring.json" || file.Name() == "dark_forest.json") {
			loc, err := loadLocation(locDir + "/" + file.Name())
			if err != nil {
				return err
			}
			locations[loc.Name] = loc
		}
	}
	return nil
}

func loadLocation(path string) (*Location, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var locConfig LocationConfig
	if err := json.Unmarshal(data, &locConfig); err != nil {
		return nil, err
	}

	loc := &Location{
		Name:             locConfig.NameKey,
		Biome:            locConfig.Biome,
		Width:            locConfig.Width,
		DayNightCycle:    locConfig.DayNightCycleSecs,
		NextLocation:     locConfig.NextLocation,
		PreviousLocation: locConfig.PreviousLocation,
		TransitionX:      locConfig.TransitionX,
		Decors:           locConfig.Decors,
		SpawnSchedule:    locConfig.SpawnSchedule,
		MaxBoars:         locConfig.MaxBoars,
		MaxHares:         locConfig.MaxHares,
		Animals:          []Animal{},
	}

	loc.Tiles = make([]Tile, loc.Width)
	for i := 0; i < loc.Width; i++ {
		surface := locConfig.Ground.DefaultTile
		if custom, ok := locConfig.Ground.Tiles[i]; ok {
			surface = custom
		}
		loc.Tiles[i] = Tile{Surface: surface, Path: "none", TrailCount: 0}
	}

	for _, objConf := range locConfig.Objects {
		obj := GameObject{
			X:          objConf.X,
			Layer:      objConf.Layer,
			Type:       objConf.TypeID,
			StageIndex: objConf.Stage,
			StageTimer: objConf.StageTimer,
			OwnerHouse: objConf.OwnerHouse,
			Resources:  make(map[string]int),
		}
		if ot, ok := objectTypes[obj.Type]; ok && obj.StageIndex < len(ot.Stages) {
			obj.State = ot.Stages[obj.StageIndex].NameKey
			for _, interactionID := range ot.Stages[obj.StageIndex].Interactions {
				if inter, ok := interactions[interactionID]; ok && inter.ResultItem != "" {
					obj.Resources[inter.ResultItem] = inter.AmountMax
				}
			}
		}
		if obj.Layer == "front" {
			loc.FrontObjects = append(loc.FrontObjects, obj)
		} else {
			loc.BackObjects = append(loc.BackObjects, obj)
		}
	}
	return loc, nil
}

// ---------- Сохранение/загрузка ----------
func saveWorld(filename string) error {
	world.mu.RLock()
	defer world.mu.RUnlock()

	saveData := SaveData{
		Locations:      locations,
		Players:        world.Players,
		GameTime:       world.GameTime,
		CurrentLocName: currentLocation.Name,
		SavedAt:        time.Now(),
	}

	data, err := json.MarshalIndent(saveData, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

func loadWorld(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var saveData SaveData
	if err := json.Unmarshal(data, &saveData); err != nil {
		return err
	}

	locations = saveData.Locations
	world.Players = saveData.Players
	world.GameTime = saveData.GameTime
	currentLocation = locations[saveData.CurrentLocName]

	for _, p := range world.Players {
		p.conn = nil
		p.send = nil
	}

	log.Printf("Мир загружен из %s (сохранён: %v)", filename, saveData.SavedAt)
	return nil
}

// ---------- Поведение животных ----------
func getAnimalConfig(animalType string) AnimalConfig {
	if cfg, ok := animalConfigs[animalType]; ok {
		return cfg
	}
	return AnimalConfig{SpeedWalk: 1.5, SpeedRun: 3.0, Damage: 0, Aggressive: false, FleeDistance: 10, AttackDistance: 1.5}
}

func updateAnimals(loc *Location, now time.Time) {
	loc.mu.Lock()
	defer loc.mu.Unlock()

	for i := range loc.Animals {
		a := &loc.Animals[i]
		if a.State == "dead" {
			if a.CorpseTimer > 0 && now.Unix() >= a.CorpseTimer {
				loc.Animals = append(loc.Animals[:i], loc.Animals[i+1:]...)
				i--
			}
			continue
		}

		cfg := getAnimalConfig(a.Type)

		var nearestPlayer *Player
		var nearestDist float64 = 9999
		world.mu.RLock()
		for _, p := range world.Players {
			if p.State == "awake" {
				dist := math.Abs(p.X - a.X)
				if dist < nearestDist {
					nearestDist = dist
					nearestPlayer = p
				}
			}
		}
		world.mu.RUnlock()

		if a.State == "sleeping" {
			if now.Unix() >= a.SleepTimer {
				a.State = "wandering"
				a.Speed = cfg.SpeedWalk
				a.WanderTimer = now.Unix() + int64(5+rand.Intn(15))
			}
			continue
		}

		if a.State == "fleeing" {
			if nearestPlayer != nil && nearestDist < cfg.FleeDistance {
				if a.X < nearestPlayer.X {
					a.X -= a.Speed * 0.1
				} else {
					a.X += a.Speed * 0.1
				}
				if a.X < 0 || a.X >= float64(loc.Width) {
					loc.Animals = append(loc.Animals[:i], loc.Animals[i+1:]...)
					i--
					continue
				}
				if nearestDist > cfg.FleeDistance+5 {
					a.State = "wandering"
					a.Speed = cfg.SpeedWalk
				}
			} else {
				a.State = "wandering"
				a.Speed = cfg.SpeedWalk
			}
			continue
		}

		if a.State == "attacking" && cfg.Aggressive {
			if nearestPlayer != nil && nearestDist <= cfg.AttackDistance {
				nearestPlayer.Hunger -= cfg.Damage
				if nearestPlayer.Hunger < 0 {
					nearestPlayer.Hunger = 0
				}
				a.State = "wandering"
				a.WanderTimer = now.Unix() + 2
			} else if nearestPlayer == nil || nearestDist > cfg.AttackDistance*2 {
				a.State = "wandering"
			}
			continue
		}

		if a.State == "eating" {
			if now.Unix() >= a.EatTimer {
				for j, obj := range loc.FrontObjects {
					if obj.X == a.EatTargetX && obj.Resources != nil {
						for _, foodType := range cfg.PreferredFood {
							if qty, ok := obj.Resources[foodType]; ok && qty > 0 {
								obj.Resources[foodType] = qty - 1
								loc.FrontObjects[j] = obj
								a.HP = min(a.HP+5, a.MaxHP)
								break
							}
						}
						break
					}
				}
				a.State = "wandering"
				a.WanderTimer = now.Unix() + int64(3+rand.Intn(10))
			}
			continue
		}

		if cfg.Aggressive && nearestPlayer != nil && nearestDist <= cfg.AttackDistance*3 {
			a.State = "attacking"
			a.TargetPlayer = nearestPlayer.ID
			a.Speed = cfg.SpeedRun
			continue
		}

		if !cfg.Aggressive && nearestPlayer != nil && nearestDist <= cfg.FleeDistance {
			a.State = "fleeing"
			a.Speed = cfg.SpeedRun
			continue
		}

		if a.State == "wandering" {
			if now.Unix() >= a.WanderTimer {
				if rand.Float64() < cfg.SleepChance {
					a.State = "sleeping"
					a.SleepTimer = now.Unix() + int64(10+rand.Intn(30))
					continue
				}
				foundFood := false
				for _, obj := range loc.FrontObjects {
					if obj.Resources != nil {
						for _, foodType := range cfg.PreferredFood {
							if qty, ok := obj.Resources[foodType]; ok && qty > 0 && math.Abs(float64(obj.X)-a.X) < 5 {
								a.State = "eating"
								a.EatTargetX = obj.X
								a.EatTimer = now.Unix() + 3
								foundFood = true
								break
							}
						}
					}
					if foundFood {
						break
					}
				}
				if foundFood {
					continue
				}
				a.MoveDir = 1
				if rand.Intn(2) == 0 {
					a.MoveDir = -1
				}
				moveDistance := float64(1 + rand.Intn(10))
				a.WanderTarget = a.X + float64(a.MoveDir)*moveDistance
				if a.WanderTarget < 0 {
					a.WanderTarget = 0
				}
				if a.WanderTarget >= float64(loc.Width) {
					a.WanderTarget = float64(loc.Width - 1)
				}
				a.WanderTimer = now.Unix() + int64(3+rand.Intn(8))
			} else {
				if math.Abs(a.X-a.WanderTarget) > 0.5 {
					if a.X < a.WanderTarget {
						a.X += a.Speed * 0.1
					} else {
						a.X -= a.Speed * 0.1
					}
				}
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------- Спавн животных ----------
func spawnAnimal(loc *Location, animalType string) {
	loc.mu.Lock()
	defer loc.mu.Unlock()

	var currentCount int
	var maxCount int
	for _, a := range loc.Animals {
		if a.Type == animalType && a.State != "dead" {
			currentCount++
		}
	}

	if animalType == "boar" {
		maxCount = loc.MaxBoars
	} else {
		maxCount = loc.MaxHares
	}

	if currentCount >= maxCount {
		return
	}

	cfg := getAnimalConfig(animalType)

	for attempts := 0; attempts < 20; attempts++ {
		x := float64(rand.Intn(loc.Width))

		nearby := false
		world.mu.RLock()
		for _, p := range world.Players {
			if math.Abs(p.X-x) < 10 {
				nearby = true
				break
			}
		}
		world.mu.RUnlock()

		if !nearby {
			animal := Animal{
				ID:    fmt.Sprintf("%s_%d", animalType, time.Now().UnixNano()),
				Type:  animalType,
				X:     x,
				HP:    cfg.HP,
				MaxHP: cfg.HP,
				State: "wandering",
				Speed: cfg.SpeedWalk,
			}
			animal.WanderTimer = time.Now().Unix() + int64(5+rand.Intn(15))
			loc.Animals = append(loc.Animals, animal)
			log.Printf("[%s] Спавнится %s на позиции %.1f", loc.Name, animalType, x)
			return
		}
	}
}

func animalSpawner() {
	for {
		if world.Paused {
			time.Sleep(1 * time.Second)
			continue
		}

		now := time.Now()

		for _, loc := range locations {
			var spawnMinute int
			fmt.Sscanf(loc.SpawnSchedule, "%d", &spawnMinute)

			if now.Minute() == spawnMinute && now.Second() == 0 {
				spawnAnimal(loc, "boar")
				spawnAnimal(loc, "hare")
			}
		}

		time.Sleep(1 * time.Second)
	}
}

// ---------- Атака и разделка ----------
func attackAnimal(player *Player, animalID string, loc *Location) (string, int, bool) {
	loc.mu.Lock()
	defer loc.mu.Unlock()

	for i, a := range loc.Animals {
		if a.ID == animalID && a.State != "dead" {
			if math.Abs(player.X-a.X) > 1.5 {
				return "", 0, false
			}

			damage := 5 + rand.Intn(10)
			a.HP -= damage

			if a.HP <= 0 {
				a.State = "dead"
				a.CorpseTimer = time.Now().Unix() + 300
				loc.Animals[i] = a
				return a.Type, 1, true
			}

			cfg := getAnimalConfig(a.Type)
			if cfg.Aggressive {
				player.Hunger -= cfg.Damage / 2
				if player.Hunger < 0 {
					player.Hunger = 0
				}
			}

			loc.Animals[i] = a
			return "", damage, true
		}
	}
	return "", 0, false
}

func butcherAnimal(player *Player, animalID string, loc *Location) (map[string]int, bool) {
	loc.mu.Lock()
	defer loc.mu.Unlock()

	for i, a := range loc.Animals {
		if a.ID == animalID && a.State == "dead" {
			rewards := make(map[string]int)
			if a.Type == "boar" {
				rewards["meat"] = 3 + rand.Intn(3)
				rewards["leather"] = 1 + rand.Intn(2)
			} else {
				rewards["meat"] = 1 + rand.Intn(2)
				rewards["fur"] = 1
			}

			loc.Animals = append(loc.Animals[:i], loc.Animals[i+1:]...)
			return rewards, true
		}
	}
	return nil, false
}

// ---------- Вспомогательные функции ----------
func surfaceModifier(surface string) float64 {
	switch surface {
	case "snow":
		return 0.5
	case "sand":
		return 0.8
	case "water":
		return 0.0
	default:
		return 1.0
	}
}

func pathModifier(path string) float64 {
	switch path {
	case "trail":
		return 1.1
	case "packed":
		return 1.3
	case "paved":
		return 1.5
	default:
		return 1.0
	}
}

func bridgeModifier(material string) float64 {
	switch material {
	case "wood":
		return 1.0
	case "stone":
		return 1.2
	default:
		return 1.0
	}
}

func (loc *Location) getTileModifiers(x int) (surfaceMod, pathMod float64, blocked bool) {
	if x < 0 || x >= loc.Width {
		return 0, 0, true
	}
	tile := loc.Tiles[x]
	if tile.Surface == "water" && tile.Bridge == nil {
		return 0, 0, true
	}
	modSurface := surfaceModifier(tile.Surface)
	modPath := pathModifier(tile.Path)
	if tile.Bridge != nil {
		modPath = bridgeModifier(tile.Bridge.Material)
	}
	return modSurface, modPath, false
}

func (loc *Location) updatePath(x int) {
	if x < 0 || x >= loc.Width {
		return
	}
	tile := &loc.Tiles[x]
	if tile.Surface == "water" {
		return
	}
	if tile.Path == "none" {
		tile.TrailCount++
		if tile.TrailCount >= 10 {
			tile.Path = "trail"
		}
	}
}

func (loc *Location) canPlaceObject(x int, layer string, objType string) bool {
	if x < 0 || x >= loc.Width {
		return false
	}
	if loc.Tiles[x].Surface == "water" && loc.Tiles[x].Bridge == nil {
		return false
	}
	var objects []GameObject
	if layer == "front" {
		objects = loc.FrontObjects
	} else {
		objects = loc.BackObjects
	}
	for _, obj := range objects {
		if obj.X == x {
			return false
		}
	}
	return true
}

func (loc *Location) addObject(obj GameObject) {
	loc.mu.Lock()
	defer loc.mu.Unlock()
	if obj.Layer == "front" {
		loc.FrontObjects = append(loc.FrontObjects, obj)
	} else {
		loc.BackObjects = append(loc.BackObjects, obj)
	}
}

func (loc *Location) processPlantGrowth(now time.Time) {
	loc.mu.Lock()
	defer loc.mu.Unlock()

	for i := range loc.FrontObjects {
		loc.updatePlantStage(&loc.FrontObjects[i], now)
	}
	for i := range loc.BackObjects {
		loc.updatePlantStage(&loc.BackObjects[i], now)
	}
}

func (loc *Location) updatePlantStage(obj *GameObject, now time.Time) {
	ot, ok := objectTypes[obj.Type]
	if !ok {
		return
	}
	if obj.StageIndex >= len(ot.Stages) {
		return
	}
	if obj.StageTimer == 0 && obj.StageIndex < len(ot.Stages)-1 && ot.Stages[obj.StageIndex].DurationSeconds > 0 {
		obj.StageTimer = now.Unix() + int64(ot.Stages[obj.StageIndex].DurationSeconds)
		return
	}
	if obj.StageTimer > 0 && now.Unix() >= obj.StageTimer && obj.StageIndex < len(ot.Stages)-1 {
		obj.StageIndex++
		obj.State = ot.Stages[obj.StageIndex].NameKey
		obj.Resources = make(map[string]int)
		for _, interactionID := range ot.Stages[obj.StageIndex].Interactions {
			if inter, ok := interactions[interactionID]; ok && inter.ResultItem != "" {
				obj.Resources[inter.ResultItem] = inter.AmountMax
			}
		}
		if obj.StageIndex < len(ot.Stages)-1 && ot.Stages[obj.StageIndex].DurationSeconds > 0 {
			obj.StageTimer = now.Unix() + int64(ot.Stages[obj.StageIndex].DurationSeconds)
		} else {
			obj.StageTimer = 0
		}
	}
}

// ---------- REST API обработчики ----------

// GET /game/status
func gameStatusHandler(w http.ResponseWriter, r *http.Request) {
	world.mu.RLock()
	defer world.mu.RUnlock()

	status := map[string]interface{}{
		"status":    "running",
		"paused":    world.Paused,
		"gameTime":  world.GameTime,
		"timeOfDay": world.TimeOfDay,
		"location":  currentLocation.Name,
		"players":   len(world.Players),
		"animals":   len(currentLocation.Animals),
		"lastSave":  world.LastSaveTime,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// POST /game/move
func gameMoveHandler(w http.ResponseWriter, r *http.Request) {
	if world.Paused {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "world paused"})
		return
	}

	var req struct {
		PlayerID  string `json:"playerId"`
		Direction int    `json:"direction"`
		Running   bool   `json:"running"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	world.mu.Lock()
	defer world.mu.Unlock()

	player, ok := world.Players[req.PlayerID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	player.Direction = req.Direction
	player.Running = req.Running

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GET /game/players
func gamePlayersHandler(w http.ResponseWriter, r *http.Request) {
	world.mu.RLock()
	defer world.mu.RUnlock()

	type PlayerInfo struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		HouseName string  `json:"houseName"`
		X         float64 `json:"x"`
		Hunger    int     `json:"hunger"`
		Thirst    int     `json:"thirst"`
		State     string  `json:"state"`
	}

	players := make([]PlayerInfo, 0)
	for _, p := range world.Players {
		players = append(players, PlayerInfo{
			ID:        p.ID,
			Name:      p.Name,
			HouseName: p.HouseName,
			X:         p.X,
			Hunger:    p.Hunger,
			Thirst:    p.Thirst,
			State:     p.State,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(players)
}

// POST /game/attack
func gameAttackHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"playerId"`
		AnimalID string `json:"animalId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	world.mu.Lock()
	player, ok := world.Players[req.PlayerID]
	if !ok {
		world.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		return
	}
	world.mu.Unlock()

	animalType, damage, hit := attackAnimal(player, req.AnimalID, currentLocation)

	if !hit {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hit":    true,
		"type":   animalType,
		"damage": damage,
	})
}

// POST /game/butcher
func gameButcherHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"playerId"`
		AnimalID string `json:"animalId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	world.mu.Lock()
	player, ok := world.Players[req.PlayerID]
	if !ok {
		world.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		return
	}
	world.mu.Unlock()

	rewards, ok := butcherAnimal(player, req.AnimalID, currentLocation)

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	world.mu.Lock()
	for item, qty := range rewards {
		player.Inventory[item] += qty
	}
	world.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"rewards": rewards,
	})
}

// GET /game/animals
func gameAnimalsHandler(w http.ResponseWriter, r *http.Request) {
	currentLocation.mu.RLock()
	defer currentLocation.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(currentLocation.Animals)
}

// POST /game/eat
func gameEatHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"playerId"`
		ItemID   string `json:"itemId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	world.mu.Lock()
	defer world.mu.Unlock()

	player, ok := world.Players[req.PlayerID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	item, ok := items[req.ItemID]
	if !ok || player.Inventory[req.ItemID] <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "item not found"})
		return
	}

	player.Inventory[req.ItemID]--
	player.Hunger += item.FoodValue
	if player.Hunger > 100 {
		player.Hunger = 100
	}
	if item.Poisonous {
		player.Hunger -= 20
		if player.Hunger < 0 {
			player.Hunger = 0
		}
	}

	if player.Inventory[req.ItemID] == 0 {
		delete(player.Inventory, req.ItemID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"hunger":    player.Hunger,
		"remaining": player.Inventory[req.ItemID],
	})
}

// POST /game/drink
func gameDrinkHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"playerId"`
		ItemID   string `json:"itemId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	world.mu.Lock()
	defer world.mu.Unlock()

	player, ok := world.Players[req.PlayerID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	item, ok := items[req.ItemID]
	if !ok || player.Inventory[req.ItemID] <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	player.Inventory[req.ItemID]--
	player.Thirst += item.WaterValue
	if player.Thirst > 100 {
		player.Thirst = 100
	}

	if player.Inventory[req.ItemID] == 0 {
		delete(player.Inventory, req.ItemID)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"thirst":  player.Thirst,
	})
}

// POST /game/chop
func gameChopHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"playerId"`
		TreeX    int    `json:"treeX"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	world.mu.Lock()
	player, ok := world.Players[req.PlayerID]
	if !ok {
		world.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		return
	}
	world.mu.Unlock()

	if math.Abs(player.X-float64(req.TreeX)) > 1.5 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "too far"})
		return
	}

	var targetObj *GameObject
	currentLocation.mu.Lock()
	for i := range currentLocation.FrontObjects {
		if currentLocation.FrontObjects[i].X == req.TreeX &&
			(currentLocation.FrontObjects[i].Type == "pine" || currentLocation.FrontObjects[i].Type == "oak") {
			targetObj = &currentLocation.FrontObjects[i]
			break
		}
	}

	if targetObj == nil {
		currentLocation.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "no tree found"})
		return
	}

	woodAmount := 1 + rand.Intn(3)
	player.Inventory["wood"] += woodAmount

	if targetObj.Resources["wood"] > 0 {
		targetObj.Resources["wood"]--
	}

	if targetObj.Resources["wood"] <= 0 {
		for i := range currentLocation.FrontObjects {
			if currentLocation.FrontObjects[i].X == req.TreeX {
				currentLocation.FrontObjects = append(currentLocation.FrontObjects[:i], currentLocation.FrontObjects[i+1:]...)
				break
			}
		}
	}
	currentLocation.mu.Unlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"wood":    woodAmount,
	})
}

// POST /game/gather
func gameGatherHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID   string `json:"playerId"`
		ObjectX    int    `json:"objectX"`
		ObjectType string `json:"objectType"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	world.mu.Lock()
	player, ok := world.Players[req.PlayerID]
	if !ok {
		world.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		return
	}
	world.mu.Unlock()

	if math.Abs(player.X-float64(req.ObjectX)) > 1.5 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "too far"})
		return
	}

	var targetObj *GameObject
	currentLocation.mu.Lock()
	for i := range currentLocation.FrontObjects {
		if currentLocation.FrontObjects[i].X == req.ObjectX && currentLocation.FrontObjects[i].Type == req.ObjectType {
			targetObj = &currentLocation.FrontObjects[i]
			break
		}
	}

	if targetObj == nil {
		currentLocation.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "object not found"})
		return
	}

	var gatheredItem string
	var amount int

	if req.ObjectType == "berry_bush" && targetObj.Resources["berry"] > 0 {
		gatheredItem = "berry"
		amount = 1 + rand.Intn(2)
		targetObj.Resources["berry"] -= amount
		if targetObj.Resources["berry"] < 0 {
			targetObj.Resources["berry"] = 0
		}
	} else if req.ObjectType == "mushroom" && targetObj.Resources["mushroom"] > 0 {
		gatheredItem = "mushroom"
		amount = 1
		targetObj.Resources["mushroom"]--
	} else {
		currentLocation.mu.Unlock()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no resources left"})
		return
	}

	player.Inventory[gatheredItem] += amount
	currentLocation.mu.Unlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"item":    gatheredItem,
		"amount":  amount,
	})
}

// POST /game/inspect (предмет в инвентаре)
func gameInspectHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"playerId"`
		ItemID   string `json:"itemId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	item, ok := items[req.ItemID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	info := map[string]interface{}{
		"id":          item.ID,
		"stackable":   item.Stackable,
		"max_stack":   item.MaxStack,
		"weight":      item.Weight,
		"food_value":  item.FoodValue,
		"water_value": item.WaterValue,
		"poisonous":   item.Poisonous,
	}

	json.NewEncoder(w).Encode(info)
}

// POST /game/examine (объект на карте)
func gameExamineHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"playerId"`
		ObjectX  int    `json:"objectX"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	world.mu.RLock()
	player, ok := world.Players[req.PlayerID]
	world.mu.RUnlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if math.Abs(player.X-float64(req.ObjectX)) > 5 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "too far"})
		return
	}

	var foundObj *GameObject
	currentLocation.mu.RLock()
	for i := range currentLocation.FrontObjects {
		if currentLocation.FrontObjects[i].X == req.ObjectX {
			foundObj = &currentLocation.FrontObjects[i]
			break
		}
	}
	if foundObj == nil {
		for i := range currentLocation.BackObjects {
			if currentLocation.BackObjects[i].X == req.ObjectX {
				foundObj = &currentLocation.BackObjects[i]
				break
			}
		}
	}
	currentLocation.mu.RUnlock()

	if foundObj == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "object not found"})
		return
	}

	info := map[string]interface{}{
		"type":      foundObj.Type,
		"state":     foundObj.State,
		"resources": foundObj.Resources,
	}

	json.NewEncoder(w).Encode(info)
}

// POST /game/craft
func gameCraftHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID  string `json:"playerId"`
		CraftType string `json:"craftType"`
		TargetX   int    `json:"targetX"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	world.mu.Lock()
	player, ok := world.Players[req.PlayerID]
	if !ok {
		world.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		return
	}
	world.mu.Unlock()

	var needed map[string]int
	var newObj GameObject
	allowed := false

	switch req.CraftType {
	case "campfire":
		needed = map[string]int{"wood": 5, "stick": 1}
		newObj = GameObject{X: req.TargetX, Layer: "front", Type: "campfire", StageIndex: 0, State: "campfire_lit"}
		allowed = true
	case "chest":
		needed = map[string]int{"wood": 10}
		newObj = GameObject{X: req.TargetX, Layer: "front", Type: "chest", StageIndex: 0, State: "chest_closed"}
		allowed = true
	case "hut":
		needed = map[string]int{"wood": 20, "leaf": 10}
		newObj = GameObject{X: req.TargetX, Layer: "back", Type: "hut", StageIndex: 0, State: "hut_standing"}
		allowed = true
	case "bridge_wood":
		if currentLocation.Tiles[req.TargetX].Surface == "water" && currentLocation.Tiles[req.TargetX].Bridge == nil {
			needed = map[string]int{"wood": 20}
			allowed = true
		}
	}

	if allowed && currentLocation.canPlaceObject(req.TargetX, newObj.Layer, req.CraftType) {
		canCraft := true
		for res, needQty := range needed {
			if player.Inventory[res] < needQty {
				canCraft = false
				break
			}
		}
		if canCraft {
			for res, needQty := range needed {
				player.Inventory[res] -= needQty
			}
			if req.CraftType == "bridge_wood" {
				currentLocation.Tiles[req.TargetX].Bridge = &Bridge{Material: "wood", HP: 50}
			} else {
				newObj.Resources = make(map[string]int)
				currentLocation.addObject(newObj)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
			return
		}
	}

	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": "cannot craft"})
}

// POST /game/poison
func gamePoisonHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayerID string `json:"playerId"`
		TreeX    int    `json:"treeX"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	world.mu.Lock()
	player, ok := world.Players[req.PlayerID]
	if !ok {
		world.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		return
	}
	world.mu.Unlock()

	if math.Abs(player.X-float64(req.TreeX)) > 1.5 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "too far"})
		return
	}

	if player.Inventory["poison"] <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no poison"})
		return
	}

	currentLocation.mu.Lock()
	defer currentLocation.mu.Unlock()

	for i := range currentLocation.FrontObjects {
		if currentLocation.FrontObjects[i].X == req.TreeX && currentLocation.FrontObjects[i].Type == "pine" {
			if currentLocation.FrontObjects[i].StageIndex < 4 {
				currentLocation.FrontObjects[i].StageIndex = 4
				currentLocation.FrontObjects[i].State = "pine_withered"
				currentLocation.FrontObjects[i].Resources = map[string]int{"rotten_wood": 3}
				currentLocation.FrontObjects[i].StageTimer = time.Now().Unix() + 600
				player.Inventory["poison"]--
				json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
				return
			}
		}
	}

	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "tree not found or already withered"})
}

// ---------- WebSocket ----------
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print(err)
		return
	}
	defer conn.Close()

	var auth struct {
		Type       string `json:"type"`
		PlayerName string `json:"playerName"`
		HouseName  string `json:"houseName"`
		Language   string `json:"language"`
	}

	if err := conn.ReadJSON(&auth); err != nil || auth.Type != "auth" {
		return
	}

	world.mu.Lock()
	if _, exists := world.Players[auth.PlayerName]; exists {
		world.mu.Unlock()
		conn.WriteJSON(map[string]string{"error": "name taken"})
		return
	}

	player := &Player{
		ID:           auth.PlayerName,
		Name:         auth.PlayerName,
		HouseName:    auth.HouseName,
		X:            50.0,
		Inventory:    make(map[string]int),
		Hunger:       80,
		Thirst:       80,
		State:        "awake",
		Relationship: make(map[string]string),
		Language:     auth.Language,
		conn:         conn,
		send:         make(chan []byte, 256),
	}
	player.Inventory["wood"] = 10
	player.Inventory["stick"] = 5
	player.Inventory["berry"] = 3

	world.Players[player.ID] = player
	world.mu.Unlock()

	go func() {
		for msg := range player.send {
			if player.conn != nil {
				player.conn.WriteMessage(websocket.TextMessage, msg)
			}
		}
	}()

	for {
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		switch msg["type"] {
		case "move":
			if !world.Paused {
				if dir, ok := msg["direction"].(float64); ok {
					player.Direction = int(dir)
				}
				if running, ok := msg["running"].(bool); ok {
					player.Running = running
				}
			}
		case "logout":
			player.State = "sleeping"
			player.Direction = 0
		case "set_language":
			if lang, ok := msg["language"].(string); ok {
				player.Language = lang
			}
		}
	}

	world.mu.Lock()
	delete(world.Players, player.ID)
	world.mu.Unlock()
}

func broadcastWorldState() {
	world.mu.RLock()
	defer world.mu.RUnlock()

	type PlayerState struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		HouseName string  `json:"houseName"`
		X         float64 `json:"x"`
		Hunger    int     `json:"hunger"`
		Thirst    int     `json:"thirst"`
		State     string  `json:"state"`
	}

	players := []PlayerState{}
	for _, p := range world.Players {
		players = append(players, PlayerState{
			ID:        p.ID,
			Name:      p.Name,
			HouseName: p.HouseName,
			X:         p.X,
			Hunger:    p.Hunger,
			Thirst:    p.Thirst,
			State:     p.State,
		})
	}

	currentLocation.mu.RLock()
	msg := map[string]interface{}{
		"type":         "state",
		"players":      players,
		"timeOfDay":    world.TimeOfDay,
		"gameTime":     world.GameTime,
		"frontObjects": currentLocation.FrontObjects,
		"backObjects":  currentLocation.BackObjects,
		"animals":      currentLocation.Animals,
		"tiles":        currentLocation.Tiles,
		"locationName": currentLocation.Name,
	}
	currentLocation.mu.RUnlock()

	data, _ := json.Marshal(msg)
	for _, p := range world.Players {
		if p.conn != nil {
			select {
			case p.send <- data:
			default:
			}
		}
	}
}

// ---------- Игровой цикл ----------
func updateGameWorld(delta time.Duration) {
	if world.Paused {
		return
	}

	world.mu.Lock()

	world.GameTime += delta.Seconds() / float64(currentLocation.DayNightCycle)
	if world.GameTime >= 1.0 {
		world.GameTime = 0.0
	}

	if world.GameTime < 0.25 {
		world.TimeOfDay = "dawn"
	} else if world.GameTime < 0.5 {
		world.TimeOfDay = "day"
	} else if world.GameTime < 0.75 {
		world.TimeOfDay = "dusk"
	} else {
		world.TimeOfDay = "night"
	}

	for _, p := range world.Players {
		if p.State != "awake" {
			continue
		}
		if p.Direction == 0 {
			continue
		}

		baseSpeed := 2.0
		if p.Running {
			baseSpeed = 5.0
		}
		if p.Hunger < 20 || p.Thirst < 20 {
			baseSpeed *= 0.5
		}

		newX := p.X + baseSpeed*float64(p.Direction)*delta.Seconds()

		if newX < 0 {
			if currentLocation.PreviousLocation != "" {
				if newLoc, ok := locations[currentLocation.PreviousLocation]; ok {
					currentLocation = newLoc
					p.X = float64(currentLocation.Width - 2)
				} else {
					newX = 0
				}
			} else {
				newX = 0
			}
		}

		if newX >= float64(currentLocation.Width-1) {
			if currentLocation.NextLocation != "" {
				if newLoc, ok := locations[currentLocation.NextLocation]; ok {
					currentLocation = newLoc
					p.X = 1.0
				} else {
					newX = float64(currentLocation.Width - 1)
				}
			} else {
				newX = float64(currentLocation.Width - 1)
			}
		}

		if newX >= 0 && newX < float64(currentLocation.Width) {
			intendedTile := int(math.Floor(newX))
			_, _, blocked := currentLocation.getTileModifiers(intendedTile)
			if !blocked {
				p.X = newX
				currentLocation.updatePath(int(math.Floor(p.X)))
			}
		}
	}
	world.mu.Unlock()

	currentLocation.processPlantGrowth(time.Now())
	updateAnimals(currentLocation, time.Now())
}

// ---------- Консоль ----------
func consoleHandler() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		cmd := scanner.Text()
		switch cmd {
		case "save":
			if err := saveWorld("save.json"); err != nil {
				log.Printf("Ошибка сохранения: %v", err)
			} else {
				log.Println("Мир сохранён в save.json")
			}
		case "load":
			if err := loadWorld("save.json"); err != nil {
				log.Printf("Ошибка загрузки: %v", err)
			} else {
				log.Println("Мир загружен из save.json")
				broadcastWorldState()
			}
		case "pause":
			world.Paused = true
			log.Println("Мир на паузе")
		case "resume":
			world.Paused = false
			log.Println("Мир возобновлён")
		case "exit":
			log.Println("Завершение работы...")
			os.Exit(0)
		default:
			log.Println("Доступные команды: save, load, pause, resume, exit")
		}
	}
}

// ---------- main ----------
func main() {
	rand.Seed(time.Now().UnixNano())

	if err := loadObjectTypes("config/object_types.json"); err != nil {
		log.Fatal("Ошибка загрузки object_types.json:", err)
	}
	if err := loadItems("config/items.json"); err != nil {
		log.Fatal("Ошибка загрузки items.json:", err)
	}
	if err := loadInteractions("config/interactions.json"); err != nil {
		log.Fatal("Ошибка загрузки interactions.json:", err)
	}
	if err := loadTranslations("static/lang"); err != nil {
		log.Println("Предупреждение: не удалось загрузить переводы:", err)
	}
	if err := loadAllLocations("config/locations"); err != nil {
		log.Fatal("Ошибка загрузки локаций:", err)
	}

	currentLocation = locations["weeping_spring"]
	world = &WorldState{
		Players:  make(map[string]*Player),
		Location: currentLocation,
		GameTime: 0.5,
		Paused:   false,
	}

	fmt.Print("Создать новый мир (new) или продолжить (load)? ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	choice := scanner.Text()
	if choice == "load" {
		if err := loadWorld("save.json"); err != nil {
			log.Println("Не удалось загрузить сохранение, создаём новый мир")
		}
	} else {
		log.Println("Создан новый мир")
	}

	go animalSpawner()
	go consoleHandler()

	moveTicker := time.NewTicker(50 * time.Millisecond)
	broadcastTicker := time.NewTicker(100 * time.Millisecond)
	hungerTicker := time.NewTicker(10 * time.Second)

	go func() {
		for range moveTicker.C {
			updateGameWorld(50 * time.Millisecond)
		}
	}()
	go func() {
		for range broadcastTicker.C {
			broadcastWorldState()
		}
	}()
	go func() {
		for range hungerTicker.C {
			world.mu.Lock()
			for _, p := range world.Players {
				if p.State == "awake" && !world.Paused {
					p.Hunger -= 1
					p.Thirst -= 1
					if p.Hunger < 0 {
						p.Hunger = 0
					}
					if p.Thirst < 0 {
						p.Thirst = 0
					}
				}
			}
			world.mu.Unlock()
		}
	}()

	// HTTP маршруты
	http.HandleFunc("/game/status", gameStatusHandler)
	http.HandleFunc("/game/move", gameMoveHandler)
	http.HandleFunc("/game/players", gamePlayersHandler)
	http.HandleFunc("/game/attack", gameAttackHandler)
	http.HandleFunc("/game/butcher", gameButcherHandler)
	http.HandleFunc("/game/animals", gameAnimalsHandler)
	http.HandleFunc("/game/eat", gameEatHandler)
	http.HandleFunc("/game/drink", gameDrinkHandler)
	http.HandleFunc("/game/chop", gameChopHandler)
	http.HandleFunc("/game/gather", gameGatherHandler)
	http.HandleFunc("/game/inspect", gameInspectHandler)
	http.HandleFunc("/game/examine", gameExamineHandler)
	http.HandleFunc("/game/craft", gameCraftHandler)
	http.HandleFunc("/game/poison", gamePoisonHandler)
	http.HandleFunc("/ws", handleWebSocket)

	// Статические файлы - ВАЖНО: правильный путь
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	log.Println("Survival server starting on :8090")
	log.Println("")
	log.Println("API endpoints:")
	log.Println("  GET  /game/status     - статус сервера")
	log.Println("  POST /game/move       - движение")
	log.Println("  GET  /game/players    - список игроков")
	log.Println("  POST /game/attack     - атака животного")
	log.Println("  POST /game/butcher    - разделка туши")
	log.Println("  GET  /game/animals    - список животных")
	log.Println("  POST /game/eat        - съесть предмет")
	log.Println("  POST /game/drink      - выпить предмет")
	log.Println("  POST /game/chop       - рубка дерева")
	log.Println("  POST /game/gather     - сбор ягод/грибов")
	log.Println("  POST /game/inspect    - осмотр предмета")
	log.Println("  POST /game/examine    - осмотр объекта на карте")
	log.Println("  POST /game/craft      - крафт предмета")
	log.Println("  POST /game/poison     - отравление дерева")
	log.Println("  WS   /ws              - WebSocket")
	log.Println("")
	log.Fatal(http.ListenAndServe(":8090", nil))
}
