(() => {
  const root = document.body;
  const matchID = root.dataset.matchId;
  if (!matchID) return;
  const replayMode = root.dataset.replay === "true";

  const ns = "http://www.w3.org/2000/svg";
  const svg = document.getElementById("pitch");
  const directions = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"];
  let state = null;
  let clockFrame = 0;
  let replayState = null;
  let replayCursor = 0;
  let replayTimer = 0;

  const point = ({ x, y }) => ({ x: 50 + x * 50, y: 55 + (y + 1) * 50 });
  const element = (name, attrs = {}) => {
    const node = document.createElementNS(ns, name);
    Object.entries(attrs).forEach(([key, value]) => node.setAttribute(key, value));
    return node;
  };
  const line = (from, to, className) => {
    const a = point(from), b = point(to);
    return element("line", { x1: a.x, y1: a.y, x2: b.x, y2: b.y, class: className });
  };
  const escapeHTML = value => String(value).replace(/[&<>"']/g, character => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"})[character]);

  function renderPitch(game) {
    svg.replaceChildren();
    svg.append(element("rect", { x: 200, y: 5, width: 100, height: 50, class: "goal-fill" }));
    svg.append(element("rect", { x: 200, y: 555, width: 100, height: 50, class: "goal-fill" }));
    for (let x = 0; x <= 8; x++) svg.append(line({x,y:0},{x,y:10},"grid-line"));
    for (let y = 0; y <= 10; y++) svg.append(line({x:0,y},{x:8,y},"grid-line"));
    svg.append(line({x:0,y:0},{x:3,y:0},"boundary"), line({x:5,y:0},{x:8,y:0},"boundary"));
    svg.append(line({x:0,y:10},{x:3,y:10},"boundary"), line({x:5,y:10},{x:8,y:10},"boundary"));
    svg.append(line({x:0,y:0},{x:0,y:10},"boundary"), line({x:8,y:0},{x:8,y:10},"boundary"));
    svg.append(line({x:3,y:-1},{x:3,y:0},"boundary"), line({x:5,y:-1},{x:5,y:0},"boundary"), line({x:3,y:-1},{x:5,y:-1},"boundary"));
    svg.append(line({x:3,y:10},{x:3,y:11},"boundary"), line({x:5,y:10},{x:5,y:11},"boundary"), line({x:3,y:11},{x:5,y:11},"boundary"));

    for (let x = 0; x <= 8; x++) for (let y = 0; y <= 10; y++) {
      const p = point({x,y}); svg.append(element("circle", {cx:p.x,cy:p.y,r:1.8,class:"node"}));
    }
    const currentRound = game.round;
    game.moves.filter(move => move.round === currentRound).forEach(move => {
      svg.append(line(move.from, move.to, `move-edge edge-${move.player}`));
      if (move.bounce) { const p=point(move.to); svg.append(element("circle",{cx:p.x,cy:p.y,r:6,class:"bounce"})); }
    });
    const ball = point(game.ball);
    svg.append(element("circle", {cx:ball.x,cy:ball.y,r:17,class:"ball-halo"}));
    svg.append(element("circle", {cx:ball.x,cy:ball.y,r:6,class:"ball"}));
  }

  function renderFeed(game) {
    const feed = document.getElementById("move-feed");
    const moves = [...game.moves].reverse().slice(0, 30);
    if (!moves.length) { feed.innerHTML = '<li class="empty">Awaiting first decision…</li>'; return; }
    feed.innerHTML = moves.map(move => {
      const timing = move.duration_ns < 1000 ? "<1µs" : move.duration_ns < 1000000 ? `${(move.duration_ns/1000).toFixed(1)}µs` : `${(move.duration_ns/1e6).toFixed(2)}ms`;
      return `<li class="${move.player}-move"><span class="move-num">${String(move.number).padStart(3,"0")}</span><span class="move-main">${move.player} · ${directions[move.direction]}${move.goal ? " · GOAL" : move.bounce ? " · BOUNCE" : ""}<small>(${move.from.x},${move.from.y}) → (${move.to.x},${move.to.y})</small></span><span class="move-time">${timing}</span></li>`;
    }).join("");
  }

  function renderTimeline(game) {
    const feed = document.getElementById("move-feed");
    const timeline = [
      ...(game.moves || []).map(move => ({...move, kind: "move"})),
      ...(game.events || []).map(event => ({...event, kind: "event"})),
    ].sort((a, b) => new Date(b.occurred_at) - new Date(a.occurred_at)).slice(0, 30);
    if (!timeline.length) {
      feed.innerHTML = '<li class="empty">Awaiting first decision...</li>';
      return;
    }
    feed.innerHTML = timeline.map(item => {
      if (item.kind === "event") {
        const label = item.type === "turn_skipped" ? "TURN SKIPPED" : "GOAL AWARDED";
        return `<li class="${item.player}-move match-event event-${item.type}"><span class="move-num">!</span><span class="move-main">${label}<small>${escapeHTML(item.message)}</small></span><span class="move-time">${item.red_score}:${item.blue_score}</span></li>`;
      }
      const timing = item.duration_ns < 1000 ? "&lt;1us" : item.duration_ns < 1000000 ? `${(item.duration_ns/1000).toFixed(1)}us` : `${(item.duration_ns/1e6).toFixed(2)}ms`;
      const detail = item.goal ? " / GOAL" : item.bounce ? " / BOUNCE" : "";
      return `<li class="${item.player}-move"><span class="move-num">${String(item.number).padStart(3,"0")}</span><span class="move-main">${item.player} / ${directions[item.direction]}${detail}<small>(${item.from.x},${item.from.y}) to (${item.to.x},${item.to.y})</small></span><span class="move-time">${timing}</span></li>`;
    }).join("");
  }

  function render(game) {
    state = game;
    document.getElementById("red-score").textContent = game.red_score;
    document.getElementById("blue-score").textContent = game.blue_score;
    document.getElementById("round").textContent = game.round;
    document.getElementById("status").textContent = game.status === "finished" ? `${game.winner} wins` : game.status;
    document.getElementById("match-message").textContent = game.last_message || "Match initialized";
    document.getElementById("move-count").textContent = `MOVE ${String(game.move_number).padStart(3,"0")}`;
    document.getElementById("telemetry-count").textContent = `${game.move_number} EDGES / ${(game.events || []).length} EVENTS`;
    const active = game.turn === "red" ? game.red_agent : game.blue_agent;
    document.getElementById("turn-agent").textContent = game.status === "finished" ? "MATCH COMPLETE" : active.name;
    const activeOwner = active.author || active.owner_email;
    document.getElementById("turn-signature").textContent = game.status === "finished"
      ? `${game.red_agent.name} vs ${game.blue_agent.name}`
      : [active.model, active.effort ? `${active.effort} effort` : "", activeOwner].filter(Boolean).join(" · ");
    const indicator = document.getElementById("turn-color"); indicator.style.background = game.turn === "red" ? "var(--red)" : "var(--blue)"; indicator.style.boxShadow = `0 0 15px ${game.turn === "red" ? "var(--red)" : "var(--blue)"}`;
    const legal = new Set((game.legal_moves || []).map(move => move.direction));
    document.getElementById("legal-count").textContent = `${legal.size} / 8`;
    document.getElementById("legal-moves").innerHTML = directions.map((name,index)=>`<span class="legal-move ${legal.has(index)?"active":""}">${name}</span>`).join("");
    renderPitch(game); renderTimeline(game);
  }

  function replaySnapshot(cursor) {
    const shownMoves = replayState.moves.slice(0, cursor);
    const last = shownMoves.at(-1);
    const cutoff = last ? new Date(last.occurred_at).getTime() : 0;
    const shownEvents = (replayState.events || []).filter(event => cursor === replayState.moves.length || new Date(event.occurred_at).getTime() <= cutoff);
    let redScore = 0;
    let blueScore = 0;
    shownMoves.forEach(move => {
      if (!move.goal) return;
      if (move.to.y === -1) redScore += 1;
      if (move.to.y === 11) blueScore += 1;
    });
    const latestScoreEvent = shownEvents.filter(event => event.type === "goal").at(-1);
    if (latestScoreEvent) {
      redScore = latestScoreEvent.red_score;
      blueScore = latestScoreEvent.blue_score;
    }
    if (cursor === replayState.moves.length) {
      redScore = replayState.red_score;
      blueScore = replayState.blue_score;
    }
    const next = replayState.moves[cursor];
    return {
      ...replayState,
      status: cursor === replayState.moves.length ? "finished" : "replay",
      winner: cursor === replayState.moves.length ? replayState.winner : "",
      red_score: redScore,
      blue_score: blueScore,
      round: last?.round || 1,
      move_number: cursor,
      ball: last?.to || {x: 4, y: 5},
      turn: next?.player || last?.player || replayState.turn,
      moves: shownMoves,
      events: shownEvents,
      legal_moves: [],
      deadline: "",
      last_message: cursor === 0
        ? "Replay ready at kickoff"
        : cursor === replayState.moves.length
          ? `${replayState.winner} wins · final result ${replayState.red_score}:${replayState.blue_score}`
          : `Replaying move ${cursor} of ${replayState.moves.length}`,
    };
  }

  function stopReplay() {
    clearTimeout(replayTimer);
    replayTimer = 0;
    document.getElementById("replay-play").textContent = replayCursor === replayState?.moves.length ? "Replay" : "Play";
  }

  function setReplayCursor(cursor) {
    replayCursor = Math.max(0, Math.min(replayState.moves.length, Number(cursor)));
    document.getElementById("replay-scrubber").value = replayCursor;
    document.getElementById("replay-position").textContent = `MOVE ${String(replayCursor).padStart(3,"0")} / ${String(replayState.moves.length).padStart(3,"0")}`;
    render(replaySnapshot(replayCursor));
    if (replayCursor === replayState.moves.length) {
      stopReplay();
    } else if (!replayTimer) {
      document.getElementById("replay-play").textContent = "Play";
    }
  }

  function playReplay() {
    if (replayTimer) { stopReplay(); return; }
    if (replayCursor === replayState.moves.length) setReplayCursor(0);
    document.getElementById("replay-play").textContent = "Pause";
    const advance = () => {
      setReplayCursor(replayCursor + 1);
      if (replayCursor < replayState.moves.length) {
        replayTimer = setTimeout(advance, Number(document.getElementById("replay-speed").value));
      }
    };
    replayTimer = setTimeout(advance, Number(document.getElementById("replay-speed").value));
  }

  function setupReplay(game) {
    replayState = game;
    const controls = document.getElementById("replay-controls");
    controls.hidden = false;
    const scrubber = document.getElementById("replay-scrubber");
    scrubber.max = game.moves.length;
    document.getElementById("replay-reset").addEventListener("click", () => { stopReplay(); setReplayCursor(0); });
    document.getElementById("replay-play").addEventListener("click", playReplay);
    document.getElementById("replay-step").addEventListener("click", () => { stopReplay(); setReplayCursor(replayCursor + 1); });
    scrubber.addEventListener("input", () => { stopReplay(); setReplayCursor(scrubber.value); });
    setReplayCursor(0);
  }

  function renderClock() {
    let remaining = state?.status === "running" ? 5000 : 0;
    if (state?.deadline && state.status === "running") remaining = Math.max(0, new Date(state.deadline).getTime() - Date.now());
    const seconds = (remaining / 1000).toFixed(3);
    document.getElementById("clock-value").textContent = seconds;
    document.getElementById("clock-bar").style.width = `${Math.min(100, remaining / 50)}%`;
    clockFrame = requestAnimationFrame(renderClock);
  }

  async function poll() {
    try {
      const response = await fetch(`/api/v1/matches/${matchID}`, { headers: { Accept: "application/json" }, cache: "no-store" });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const game = await response.json();
      if (replayMode) { setupReplay(game); return; }
      render(game);
      if (game.status !== "finished") setTimeout(poll, 250);
    } catch (error) {
      document.getElementById("match-message").textContent = `Connection lost: ${error.message}`;
      setTimeout(poll, 1500);
    }
  }
  poll(); clockFrame = requestAnimationFrame(renderClock);
  window.addEventListener("beforeunload", () => cancelAnimationFrame(clockFrame));
})();
