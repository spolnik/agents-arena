def choose_move(state):
    """Drive toward the opponent's goal while preferring useful bounces."""
    moves = state["legal_moves"]
    north = state["attacks"] == "north"
    best = moves[0]
    best_score = -1000000

    for move in moves:
        x = move["to"]["x"]
        y = move["to"]["y"]
        scores_for_us = (north and y == -1) or (not north and y == 11)
        scores_against_us = (north and y == 11) or (not north and y == -1)

        center_distance = x - 4
        if center_distance < 0:
            center_distance = -center_distance

        progress = -y if north else y
        score = progress * 100 - center_distance * 8
        if move["bounce"]:
            score += 35
        if scores_for_us:
            score += 100000
        if scores_against_us:
            score -= 100000

        if score > best_score:
            best = move
            best_score = score

    return best["direction"]
