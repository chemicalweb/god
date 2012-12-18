
Node = function(g, data) {
  var that = new Object();
	that.data = data;
	that.gob_addr = data.Addr;
	var m = /^(.*):(.*)$/.exec(that.gob_addr);
	that.json_addr = m[1] + ":" + (1 + parseInt(m[2]));
	that.angle = Big(3 * Math.PI / 2).plus($.base64.decode2big(data.Pos).div(g.maxPos).times(Math.PI * 2)).toFixed();
	that.hexpos = "";
	_.each($.base64.decode2b(data.Pos), function(b) {
	  var thishex = b.toString(16);
		while (thishex.length < 2) {
		  thishex = "0" + thishex;
		}
	  that.hexpos += thishex;
	});
	that.x = g.cx + Math.cos(that.angle) * g.r;
	that.y = g.cy + Math.sin(that.angle) * g.r;
	while (that.hexpos.length < 32) {
	  that.hexpos = "0" + that.hexpos;
	}
	return that;
}

God = function() {
  var that = new Object();
	that.maxPos = Big(1).times(Big(256).pow(16));
	that.cx = 1000;
	that.cy = 1000;
	that.r = 800;
  that.drawChord = function() {
		var stage = new createjs.Stage(document.getElementById("chord"));
		stage.enableMouseOver();

		var circle = new createjs.Shape();
		circle.graphics.beginStroke(createjs.Graphics.getRGB(0,0,0)).drawCircle(that.cx, that.cy, that.r);
		stage.addChild(circle);

    var dash = new createjs.Shape();
		dash.graphics.beginStroke(createjs.Graphics.getRGB(0,0,0)).moveTo(that.cx, that.cy - that.r - 30).lineTo(that.cx, that.cy - that.r + 30);
		stage.addChild(dash);

    _.each(that.routes, function(route) {
		  var click = function() {
				window.location = "http://" + route.json_addr;
			};
			var mouseover = function() {
				$("#chord").css({cursor: "pointer"});
			};
			var mouseout = function() {
				$("#chord").css({cursor: "default"}); 
			};
		  var spot = new createjs.Shape();
			spot.graphics.beginStroke(createjs.Graphics.getRGB(0,0,0)).beginFill(createjs.Graphics.getRGB(0,0,0)).drawCircle(route.x, route.y, 10);
			spot.onClick = click;
			spot.onMouseOver = mouseover;
			spot.onMouseOut = mouseout;
			stage.addChild(spot);
			var label = new createjs.Text(route.hexpos + "@" + route.gob_addr, "bold 25px Courier");
			label.onClick = click;
			label.onMouseOver = mouseover;
			label.onMouseOut = mouseout;
			label.x = route.x + 30;
			label.y = route.y - 10;
			stage.addChild(label);
		});

		stage.update();

		if (that.node != null) {
			$("#node_json_addr").text(that.node.json_addr);
			$("#node_gob_addr").text(that.node.gob_addr);
			$("#node_pos").text(that.node.hexpos);
			$("#node_owned_keys").text(that.node.data.OwnedEntries);
			$("#node_held_keys").text(that.node.data.HeldEntries);
		}
	};
	that.routes = [];
	that.node = null;
	that.start = function() {
		that.socket = $.websocket("ws://" + document.location.hostname + ":" + document.location.port + "/ws", {
			open: function() { 
				console.log("socket opened");
			},
			close: function() { 
				console.log("socket closed");
			},
			events: {
				RingChange: function(e) {
					that.routes = [];
					_.each(e.data.routes, function(r) {
						that.routes.push(Node(that, r));
					});
					that.node = Node(that, e.data.description);
					that.drawChord();
				},
				Sync: function(e) {
					console.log(e.data);
				},
				Clean: function(e) {
					console.log(e.data);
				},
			},
		});
	};
	return that;
};

g = new God();

$(function() {
  g.start()
	g.drawChord();
});

