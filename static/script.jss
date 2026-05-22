let ws = null;
let myName = null;
let myHouse = null;
let myX = 0;
let players = {};
let frontObjects = [];
let backObjects = [];
let tiles = [];
let animals = [];
let selectedTileX = null;
let selectedPlayer = null;
let selectedPoisonTarget = null;
let selectedAnimal = null;
let cameraX = 0;
let myInventory = {};
let currentLocation = "";
let currentLang = "ru";
let translations = {};

const canvas = document.getElementById('gameCanvas');
const ctx = canvas.getContext('2d');

async function loadLanguage(lang) {
    currentLang = lang;
    const response = await fetch(`lang/${lang}.json`);
    translations = await response.json();
    updateUITexts();
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'set_language', language: lang }));
    }
}

function updateUITexts() {
    document.getElementById('hungerLabel').innerText = translations['hunger'] || 'Голод';
    document.getElementById('thirstLabel').innerText = translations['thirst'] || 'Жажда';
    document.getElementById('inventoryTitle').innerHTML = translations['inventory'] || '📦 Инвентарь';
    document.getElementById('craftTitle').innerHTML = translations['craft'] || '🔨 Крафт';
    document.getElementById('craftHint').innerText = translations['craft_hint'] || 'Кликни на тайл, затем на кнопку';
    document.getElementById('poisonTitle').innerHTML = translations['poison_title'] || '🧪 Отравление дерева';
    document.getElementById('poisonHint').innerText = translations['poison_hint'] || 'Если есть ядовитый отвар, кликни на дерево';
    document.getElementById('craftCampfire').innerText = translations['craft_campfire'] || '🔥 Костёр (5 дерева, 1 палка)';
    document.getElementById('craftChest').innerText = translations['craft_chest'] || '📦 Ящик (10 дерева)';
    document.getElementById('craftHut').innerText = translations['craft_hut'] || '🏠 Шалаш (20 дерева, 10 листьев)';
    document.getElementById('craftBridge').innerText = translations['craft_bridge'] || '🌉 Деревянный мост (20 дерева, на воду)';
    document.getElementById('poisonBtn').innerText = translations['poison_btn'] || '☠️ Отравить дерево';
}

function getLocalizedText(key) {
    return translations[key] || key;
}

document.getElementById('authBtn').onclick = () => {
    myName = document.getElementById('playerName').value.trim();
    myHouse = document.getElementById('houseName').value.trim();
    if (!myName || !myHouse) return;
    
    ws = new WebSocket('ws://localhost:8090/ws');
    ws.onopen = () => {
        ws.send(JSON.stringify({
            type: 'auth',
            playerName: myName,
            houseName: myHouse,
            language: currentLang
        }));
    };
    
    ws.onmessage = (e) => {
        const msg = JSON.parse(e.data);
        if (msg.error) {
            alert(msg.error);
            return;
        }
        if (msg.type === 'state') {
            players = {};
            msg.players.forEach(p => { players[p.id] = p; });
            frontObjects = msg.frontObjects || [];
            backObjects = msg.backObjects || [];
            tiles = msg.tiles || [];
            animals = msg.animals || [];
            currentLocation = getLocalizedText(msg.locationName) || msg.locationName;
            const me = players[myName];
            if (me) {
                myX = me.x;
                document.getElementById('hunger').innerText = me.hunger;
                document.getElementById('thirst').innerText = me.thirst;
            }
            document.getElementById('timeOfDay').innerText = translations[msg.timeOfDay] || msg.timeOfDay;
            document.getElementById('locationName').innerText = currentLocation;
            updateInventoryDisplay();
        }
        if (msg.type === 'inspect_result') {
            const target = msg.target;
            alert(`📋 ${getLocalizedText('player')}: ${target.name}\n🏠 ${getLocalizedText('house')}: ${target.house}\n💤 ${getLocalizedText('state')}: ${getLocalizedText(target.state) || target.state}\n🍖 ${getLocalizedText('hunger')}: ${target.hunger}\n💧 ${getLocalizedText('thirst')}: ${target.thirst}`);
        }
        if (msg.type === 'inventory_update') {
            myInventory = msg.inventory;
            document.getElementById('hunger').innerText = msg.hunger;
            document.getElementById('thirst').innerText = msg.thirst;
            updateInventoryDisplay();
        }
    };
    
    ws.onclose = () => {
        if (ws) ws.send(JSON.stringify({type:'logout'}));
    };
    
    document.getElementById('authPanel').style.display = 'none';
    document.getElementById('gamePanel').style.display = 'flex';
    
    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('keyup', handleKeyUp);
    window.addEventListener('beforeunload', () => {
        if (ws) ws.send(JSON.stringify({type:'logout'}));
    });
    
    canvas.addEventListener('click', handleCanvasClick);
    
    document.querySelectorAll('[data-craft]').forEach(btn => {
        btn.onclick = () => {
            if (selectedTileX === null) {
                alert(getLocalizedText('select_tile_first'));
                return;
            }
            const craftType = btn.getAttribute('data-craft');
            fetch('/game/craft', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    playerId: myName,
                    craftType: craftType,
                    targetX: selectedTileX
                })
            });
            selectedTileX = null;
            document.getElementById('craftStatus').innerText = '';
        };
    });
    
    document.getElementById('poisonBtn').onclick = () => {
        if (selectedPoisonTarget === null) {
            alert(getLocalizedText('select_tree_first'));
            return;
        }
        if (myInventory['poison'] && myInventory['poison'] > 0) {
            fetch('/game/poison', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    playerId: myName,
                    treeX: selectedPoisonTarget
                })
            });
            selectedPoisonTarget = null;
        } else {
            alert(getLocalizedText('no_poison'));
        }
    };
    
    requestAnimationFrame(draw);
};

document.querySelectorAll('.lang-btn').forEach(btn => {
    btn.onclick = () => {
        const lang = btn.getAttribute('data-lang');
        loadLanguage(lang);
    };
});

document.getElementById('langGameBtn').onclick = () => {
    const newLang = currentLang === 'ru' ? 'en' : 'ru';
    loadLanguage(newLang);
};

async function updateInventoryDisplay() {
    const container = document.getElementById('invList');
    container.innerHTML = '';
    if (Object.keys(myInventory).length === 0) {
        const li = document.createElement('li');
        li.textContent = getLocalizedText('empty');
        container.appendChild(li);
        return;
    }
    const icons = {
        wood: '🪵', stone: '🪨', mushroom: '🍄', berry: '🫐',
        stick: '🥢', acorn: '🌰', leaf: '🍃', rotten_mushroom: '💀🍄',
        rotten_wood: '💀🪵', poison: '🧪', meat: '🍖', leather: '👝', fur: '🐇'
    };
    for (let [item, qty] of Object.entries(myInventory)) {
        const li = document.createElement('li');
        const itemName = getLocalizedText(item) || item;
        li.innerHTML = `${icons[item] || '📦'} ${itemName}: ${qty}`;
        if (item === 'mushroom' || item === 'berry' || item === 'acorn' || item === 'meat') {
            const eatBtn = document.createElement('button');
            eatBtn.innerText = getLocalizedText('eat');
            eatBtn.style.marginLeft = '10px';
            eatBtn.style.fontSize = '10px';
            eatBtn.onclick = () => {
                fetch('/game/eat', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ playerId: myName, itemId: item })
                }).then(() => updateInventoryDisplay());
            };
            li.appendChild(eatBtn);
        }
        container.appendChild(li);
    }
}

function handleCanvasClick(e) {
    const rect = canvas.getBoundingClientRect();
    const scaleX = canvas.width / rect.width;
    const clickX = (e.clientX - rect.left) * scaleX;
    const worldX = cameraX + clickX / 32;
    const tileX = Math.floor(worldX);
    
    if (tileX >= 0 && tileX < 100) {
        let clickedPlayer = null;
        for (let id in players) {
            const p = players[id];
            const screenX = (p.x - cameraX) * 32;
            if (Math.abs(clickX - screenX) < 32) {
                clickedPlayer = p;
                break;
            }
        }
        if (clickedPlayer) {
            selectedPlayer = clickedPlayer;
            showSocialMenu(clickedPlayer);
        } else {
            let clickedAnimal = null;
            for (let a of animals) {
                const screenX = (a.x - cameraX) * 32;
                if (Math.abs(clickX - screenX) < 32) {
                    clickedAnimal = a;
                    break;
                }
            }
            if (clickedAnimal) {
                selectedAnimal = clickedAnimal;
                showAnimalMenu(clickedAnimal);
            } else {
                selectedTileX = tileX;
                const obj = [...frontObjects, ...backObjects].find(o => o.x === tileX);
                if (obj && obj.resources && Object.keys(obj.resources).length > 0) {
                    showCollectMenu(obj, tileX, obj.layer);
                } else if (obj && obj.type === 'pine' && obj.state !== 'pine_withered' && obj.state !== 'pine_sapling') {
                    selectedPoisonTarget = tileX;
                    document.getElementById('craftStatus').innerHTML = `⚠️ ${getLocalizedText('tree_selected')} (${tileX})`;
                    setTimeout(() => {
                        if (document.getElementById('craftStatus').innerHTML.includes('⚠️')) {
                            document.getElementById('craftStatus').innerHTML = '';
                        }
                    }, 3000);
                } else {
                    document.getElementById('craftStatus').innerHTML = `✅ ${getLocalizedText('tile_selected')} ${tileX}`;
                    setTimeout(() => {
                        if (document.getElementById('craftStatus').innerHTML.includes('✅')) {
                            document.getElementById('craftStatus').innerHTML = '';
                        }
                    }, 2000);
                }
            }
        }
    }
}

function showAnimalMenu(animal) {
    const menu = document.getElementById('collectMenu');
    const title = document.getElementById('collectTitle');
    const buttonsDiv = document.getElementById('collectButtons');
    title.innerText = `🐗 ${getLocalizedText(animal.type)} (HP: ${animal.hp}/${animal.maxHp})`;
    buttonsDiv.innerHTML = '';
    
    if (animal.state !== 'dead') {
        const attackBtn = document.createElement('button');
        attackBtn.innerHTML = `⚔️ ${getLocalizedText('attack')}`;
        attackBtn.onclick = () => {
            fetch('/game/attack', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ playerId: myName, animalId: animal.id })
            }).then(() => menu.style.display = 'none');
        };
        buttonsDiv.appendChild(attackBtn);
    } else {
        const butcherBtn = document.createElement('button');
        butcherBtn.innerHTML = `🔪 ${getLocalizedText('butcher')}`;
        butcherBtn.onclick = () => {
            fetch('/game/butcher', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ playerId: myName, animalId: animal.id })
            }).then(() => menu.style.display = 'none');
        };
        buttonsDiv.appendChild(butcherBtn);
    }
    
    const cancelBtn = document.createElement('button');
    cancelBtn.innerText = getLocalizedText('cancel') || '❌ Отмена';
    cancelBtn.onclick = () => menu.style.display = 'none';
    buttonsDiv.appendChild(cancelBtn);
    menu.style.display = 'block';
}

function showCollectMenu(obj, x, layer) {
    const menu = document.getElementById('collectMenu');
    const title = document.getElementById('collectTitle');
    const buttonsDiv = document.getElementById('collectButtons');
    const objName = getLocalizedText(obj.type) || obj.type;
    const stageName = getLocalizedText(obj.state) || obj.state;
    title.innerText = `🌿 ${objName} (${stageName})`;
    buttonsDiv.innerHTML = '';
    
    if (obj.type === 'berry_bush' && obj.resources['berry'] > 0) {
        const btn = document.createElement('button');
        btn.innerHTML = `🫐 ${getLocalizedText('gather_berry')}`;
        btn.onclick = () => {
            fetch('/game/gather', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ playerId: myName, objectX: x, objectType: 'berry_bush' })
            }).then(() => menu.style.display = 'none');
        };
        buttonsDiv.appendChild(btn);
    }
    
    if (obj.type === 'mushroom' && obj.resources['mushroom'] > 0) {
        const btn = document.createElement('button');
        btn.innerHTML = `🍄 ${getLocalizedText('gather_mushroom')}`;
        btn.onclick = () => {
            fetch('/game/gather', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ playerId: myName, objectX: x, objectType: 'mushroom' })
            }).then(() => menu.style.display = 'none');
        };
        buttonsDiv.appendChild(btn);
    }
    
    if ((obj.type === 'pine' || obj.type === 'oak') && obj.resources['wood'] > 0) {
        const btn = document.createElement('button');
        btn.innerHTML = `🪵 ${getLocalizedText('chop_wood')}`;
        btn.onclick = () => {
            fetch('/game/chop', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ playerId: myName, treeX: x })
            }).then(() => menu.style.display = 'none');
        };
        buttonsDiv.appendChild(btn);
    }
    
    const cancelBtn = document.createElement('button');
    cancelBtn.innerText = getLocalizedText('cancel') || '❌ Отмена';
    cancelBtn.onclick = () => menu.style.display = 'none';
    buttonsDiv.appendChild(cancelBtn);
    menu.style.display = 'block';
}

function showSocialMenu(player) {
    const menu = document.getElementById('socialMenu');
    const title = document.getElementById('socialTitle');
    let stateIcon = '';
    if (player.state === 'awake') stateIcon = '😃';
    else if (player.state === 'sleeping') stateIcon = '💤';
    else stateIcon = '😵';
    title.innerText = `${stateIcon} ${player.name} (${player.house})`;
    const buttonsDiv = document.getElementById('socialButtons');
    buttonsDiv.innerHTML = '';
    
    const inspectBtn = document.createElement('button');
    inspectBtn.innerHTML = `🔍 ${getLocalizedText('inspect') || 'Изучить'}`;
    inspectBtn.onclick = () => {
        alert(`Игрок: ${player.name}\nДом: ${player.house}\nСостояние: ${player.state}\nГолод: ${player.hunger}\nЖажда: ${player.thirst}`);
        menu.style.display = 'none';
    };
    buttonsDiv.appendChild(inspectBtn);
    
    const cancelBtn = document.createElement('button');
    cancelBtn.innerText = getLocalizedText('close') || '❌ Закрыть';
    cancelBtn.onclick = () => menu.style.display = 'none';
    buttonsDiv.appendChild(cancelBtn);
    
    menu.style.display = 'block';
}

let moveState = { left: false, right: false, running: false };

function handleKeyDown(e) {
    if (e.key === 'a' || e.key === 'ArrowLeft') moveState.left = true;
    if (e.key === 'd' || e.key === 'ArrowRight') moveState.right = true;
    if (e.key === 'Shift') moveState.running = true;
    updateDirection();
}

function handleKeyUp(e) {
    if (e.key === 'a' || e.key === 'ArrowLeft') moveState.left = false;
    if (e.key === 'd' || e.key === 'ArrowRight') moveState.right = false;
    if (e.key === 'Shift') moveState.running = false;
    updateDirection();
}

function updateDirection() {
    let dir = 0;
    if (moveState.left) dir = -1;
    if (moveState.right) dir = 1;
    fetch('/game/move', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ playerId: myName, direction: dir, running: moveState.running })
    });
}

function draw() {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    
    const me = players[myName];
    if (me) {
        cameraX = me.x - canvas.width / 2 / 32;
    }
    if (cameraX < 0) cameraX = 0;
    if (cameraX > 100 - canvas.width / 32) cameraX = 100 - canvas.width / 32;
    
    const startTile = Math.floor(cameraX);
    const endTile = Math.min(99, startTile + Math.ceil(canvas.width / 32));
    
    for (let x = startTile; x <= endTile; x++) {
        const screenX = (x - cameraX) * 32;
        
        let color = '#7c9c5e';
        if (tiles[x] && tiles[x].surface === 'water') color = '#3b7c9c';
        if (tiles[x] && tiles[x].surface === 'dark_soil') color = '#3a4a3a';
        if (tiles[x] && tiles[x].path === 'trail') color = '#b89a6a';
        if (tiles[x] && tiles[x].path === 'packed') color = '#a07a4a';
        ctx.fillStyle = color;
        ctx.fillRect(screenX, 100, 32, 32);
        ctx.strokeStyle = '#5a7a3a';
        ctx.strokeRect(screenX, 100, 32, 32);
        
        if (tiles[x] && tiles[x].bridge) {
            ctx.fillStyle = '#8b5a2b';
            ctx.fillRect(screenX, 100, 32, 10);
            ctx.fillStyle = '#6b4a2b';
            ctx.fillRect(screenX, 110, 32, 4);
        }
        
        const objF = frontObjects.find(o => o.x === x);
        if (objF) {
            drawPlant(ctx, screenX, objF);
        }
        
        const objB = backObjects.find(o => o.x === x);
        if (objB) {
            drawPlant(ctx, screenX, objB);
        }
    }
    
    for (let a of animals) {
        const screenX = (a.x - cameraX) * 32;
        if (screenX + 32 < 0 || screenX > canvas.width) continue;
        
        if (a.state === 'dead') {
            ctx.fillStyle = '#555555';
            ctx.fillRect(screenX+8, 88, 16, 12);
            ctx.fillStyle = '#aa0000';
            ctx.fillText('💀', screenX+12, 96);
        } else {
            if (a.type === 'boar') {
                ctx.fillStyle = '#8B5A2B';
                ctx.fillRect(screenX+8, 88, 16, 12);
                ctx.fillStyle = '#5A3A1A';
                ctx.fillRect(screenX+4, 92, 4, 8);
                ctx.fillRect(screenX+24, 92, 4, 8);
            } else {
                ctx.fillStyle = '#C0A080';
                ctx.fillRect(screenX+8, 88, 16, 12);
                ctx.fillStyle = '#FFFFFF';
                ctx.fillRect(screenX+12, 86, 4, 4);
                ctx.fillRect(screenX+20, 86, 4, 4);
            }
            if (a.state === 'sleeping') {
                ctx.fillStyle = '#ffffff';
                ctx.fillText('💤', screenX+20, 84);
            }
        }
    }
    
    for (let id in players) {
        const p = players[id];
        const screenX = (p.x - cameraX) * 32;
        if (screenX + 32 < 0 || screenX > canvas.width) continue;
        
        let color = '#44ffaa';
        if (p.state === 'sleeping') color = '#888888';
        if (p.name === myName) color = '#ffaa44';
        
        ctx.fillStyle = color;
        ctx.fillRect(screenX+8, 84, 16, 24);
        ctx.fillStyle = '#000000';
        ctx.fillRect(screenX+12, 90, 4, 4);
        ctx.fillRect(screenX+20, 90, 4, 4);
        ctx.fillStyle = '#ffffff';
        ctx.font = '12px sans-serif';
        ctx.fillText(p.name, screenX+4, 82);
        
        if (p.state === 'sleeping') {
            ctx.fillStyle = '#ffffff';
            ctx.fillText('💤', screenX+24, 80);
        }
    }
}

function drawPlant(ctx, x, obj) {
    const y = 88;
    const stageName = obj.state;
    
    if (obj.type === 'mushroom') {
        if (stageName === 'mushroom_small') {
            ctx.fillStyle = '#b87a5a';
            ctx.fillRect(x+14, y+8, 4, 4);
        } else if (stageName === 'mushroom_ripe') {
            ctx.fillStyle = '#d48c5c';
            ctx.fillRect(x+12, y+4, 8, 12);
            ctx.fillStyle = '#bc6c3c';
            ctx.beginPath();
            ctx.ellipse(x+16, y+2, 6, 4, 0, 0, Math.PI*2);
            ctx.fill();
        } else if (stageName === 'mushroom_rotten') {
            ctx.fillStyle = '#6a4a3a';
            ctx.fillRect(x+12, y+4, 8, 12);
            ctx.fillStyle = '#4a2a1a';
            ctx.beginPath();
            ctx.ellipse(x+16, y+2, 6, 4, 0, 0, Math.PI*2);
            ctx.fill();
        }
    } else if (obj.type === 'berry_bush') {
        if (stageName === 'bush_sapling') {
            ctx.fillStyle = '#6a9a4a';
            ctx.fillRect(x+14, y+8, 4, 8);
        } else if (stageName === 'bush_young') {
            ctx.fillStyle = '#5a8a3a';
            ctx.fillRect(x+10, y+4, 12, 12);
        } else if (stageName === 'bush_ripe') {
            ctx.fillStyle = '#4a8c3a';
            ctx.fillRect(x+8, y+4, 16, 12);
            ctx.fillStyle = '#cc3a3a';
            for (let i = 0; i < 3; i++) {
                ctx.fillRect(x+12+i*4, y+2, 2, 2);
            }
        } else if (stageName === 'bush_old') {
            ctx.fillStyle = '#5a7a3a';
            ctx.fillRect(x+8, y+4, 16, 12);
            ctx.fillStyle = '#aa3a3a';
            for (let i = 0; i < 2; i++) {
                ctx.fillRect(x+14+i*4, y+2, 2, 2);
            }
        }
    } else if (obj.type === 'pine') {
        if (stageName === 'pine_sapling') {
            ctx.fillStyle = '#8a6a3a';
            ctx.fillRect(x+15, y+4, 2, 12);
            ctx.fillStyle = '#6aaa4a';
            ctx.beginPath();
            ctx.ellipse(x+16, y+2, 4, 4, 0, 0, Math.PI*2);
            ctx.fill();
        } else if (stageName === 'pine_young') {
            ctx.fillStyle = '#7a5a2a';
            ctx.fillRect(x+14, y+0, 4, 16);
            ctx.fillStyle = '#5aaa4a';
            ctx.beginPath();
            ctx.ellipse(x+16, y-2, 8, 8, 0, 0, Math.PI*2);
            ctx.fill();
        } else if (stageName === 'pine_mature') {
            ctx.fillStyle = '#6a4a1a';
            ctx.fillRect(x+13, y-4, 6, 20);
            ctx.fillStyle = '#4a9a3a';
            ctx.beginPath();
            ctx.ellipse(x+16, y-6, 10, 10, 0, 0, Math.PI*2);
            ctx.fill();
        } else if (stageName === 'pine_old') {
            ctx.fillStyle = '#5a3a1a';
            ctx.fillRect(x+13, y-6, 6, 22);
            ctx.fillStyle = '#3a8a2a';
            ctx.beginPath();
            ctx.ellipse(x+16, y-8, 12, 12, 0, 0, Math.PI*2);
            ctx.fill();
        } else if (stageName === 'pine_withered') {
            ctx.fillStyle = '#6a4a2a';
            ctx.fillRect(x+13, y-4, 6, 20);
            ctx.fillStyle = '#7a6a3a';
            ctx.beginPath();
            ctx.ellipse(x+16, y-6, 10, 10, 0, 0, Math.PI*2);
            ctx.fill();
            ctx.fillStyle = '#aa8a4a';
            ctx.fillText('💀', x+12, y-8);
        }
    } else if (obj.type === 'oak') {
        ctx.fillStyle = '#5a3a1a';
        ctx.fillRect(x+12, y-8, 8, 24);
        ctx.fillStyle = '#3a8a2a';
        ctx.beginPath();
        ctx.ellipse(x+16, y-12, 14, 14, 0, 0, Math.PI*2);
        ctx.fill();
        if (stageName === 'oak_old') {
            ctx.fillStyle = '#2a6a1a';
            ctx.beginPath();
            ctx.ellipse(x+16, y-12, 16, 16, 0, 0, Math.PI*2);
            ctx.fill();
        }
    } else if (obj.type === 'campfire') {
        ctx.fillStyle = '#8b5a2b';
        ctx.fillRect(x+8, y+8, 16, 4);
        ctx.fillStyle = '#ff6a2a';
        ctx.beginPath();
        ctx.moveTo(x+16, y);
        ctx.lineTo(x+12, y+8);
        ctx.lineTo(x+20, y+8);
        ctx.fill();
    } else if (obj.type === 'chest') {
        ctx.fillStyle = '#b87a3a';
        ctx.fillRect(x+8, y+0, 16, 12);
        ctx.fillStyle = '#d89a5a';
        ctx.fillRect(x+14, y-4, 4, 4);
    } else if (obj.type === 'hut') {
        ctx.fillStyle = '#8b5a2b';
        ctx.fillRect(x+4, y+0, 24, 12);
        ctx.fillStyle = '#6b3a1a';
        ctx.beginPath();
        ctx.moveTo(x+2, y);
        ctx.lineTo(x+16, y-12);
        ctx.lineTo(x+30, y);
        ctx.fill();
    }
}

window.onclick = function(e) {
    if (!e.target.closest('#collectMenu')) {
        document.getElementById('collectMenu').style.display = 'none';
    }
    if (!e.target.closest('#socialMenu')) {
        document.getElementById('socialMenu').style.display = 'none';
    }
};

loadLanguage('ru');