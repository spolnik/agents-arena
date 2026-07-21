def choose_move(state):
    """Score immediately, otherwise move toward the opponent's goal."""
    moves = state["legal_moves"]

    for move in moves:
        if move["goal"]:
            return move["direction"]

    north = state["attacks"] == "north"
    best = moves[0]
    for move in moves:
        if north and move["to"]["y"] < best["to"]["y"]:
            best = move
        if not north and move["to"]["y"] > best["to"]["y"]:
            best = move
    return best["direction"]
