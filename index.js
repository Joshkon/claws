const express = require('express')
const path = require('path');
const { config } = require('process');
const app = express()
var WebSocketClient = require('websocket').client;
var client = new WebSocketClient();
var gateway;

app.get('/', (req, res) => {
    res.sendFile(path.join(__dirname+'/control.html'));
})

app.get('/control/start', (req, res) => {
    if(!gateway) return;

    gateway.send(JSON.stringify({"command": "start", "gameTime": 30, "letGrab": 1, "grabPower": 30, "topPower": 28, "movePower": 26, "maxPower": 30, "topHeight": 20, "lineLength": 30, "xMotor": 8, "zMotor": 8, "yMotor": 8}))
    res.send({success: true})
})

app.get('/control/move', (req, res) => {
    if(!gateway) return;

    let action = parseInt(req.query.direction)
    gateway.send(JSON.stringify({"command": "move", "action": action}))
    res.send({success: true})
})

app.get('/control/grab', (req, res) => {
    if(!gateway) return;

    gateway.send(JSON.stringify({"command": "grab"}))
    res.send({success: true})
})

client.on('connect', function(connection) {
    gateway = connection;

    connection.on('message', function(message) {
        console.log(message)
    });

    console.log("Succesvol verbonden met grijpmachine gateway")
})

client.on('connectFailed', function(error) {
    console.log('Connect Error: ' + error.toString());
});

client.connect('ws://127.0.0.1:8088/websocket', 'echo-protocol');
app.listen(80, () => {
    console.log(`Grijpmachine simple app running op port 80`)
})