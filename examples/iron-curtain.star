def point_matches(point, x, y):
    return point["x"] == x and point["y"] == y

def choose_move(state):
    """Clear danger, occupy connected points, and squeeze play to the walls."""
    moves = state["legal_moves"]
    north = state["attacks"] == "north"
    ball_y = state["ball"]["y"]
    in_danger = ball_y >= 5 if north else ball_y <= 5
    best = moves[0]
    best_score = -1000000

    for move in moves:
        x = move["to"]["x"]
        y = move["to"]["y"]
        scores_for_us = (north and y == -1) or (not north and y == 11)
        scores_against_us = (north and y == 11) or (not north and y == -1)

        clearance = 10 - y if north else y
        edge_pressure = x - 4
        if edge_pressure < 0:
            edge_pressure = -edge_pressure

        connections = 0
        for edge in state["path"]:
            if point_matches(edge[0], x, y) or point_matches(edge[1], x, y):
                connections += 1

        score = clearance * (110 if in_danger else 45)
        score += edge_pressure * 22
        score += connections * 28
        if move["bounce"]:
            score += 90
        if scores_for_us:
            score += 100000
        if scores_against_us:
            score -= 100000

        if score > best_score:
            best = move
            best_score = score

    return best["direction"]
