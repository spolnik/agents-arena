"""A two-ply adversarial paper-soccer agent.

Apex Search reconstructs the current round's graph, simulates bounce ownership,
and chooses the move with the best result against the opponent's strongest reply.
"""

DIRECTIONS = [
    (0, -1),
    (1, -1),
    (1, 0),
    (1, 1),
    (0, 1),
    (-1, 1),
    (-1, 0),
    (-1, -1),
]
WIN = 1000000
INF = 2000000

def point_key(x, y):
    return (x, y)

def edge_key(ax, ay, bx, by):
    if ax > bx or (ax == bx and ay > by):
        return (bx, by, ax, ay)
    return (ax, ay, bx, by)

def add_edge(used, degree, ax, ay, bx, by):
    used[edge_key(ax, ay, bx, by)] = True
    a = point_key(ax, ay)
    b = point_key(bx, by)
    degree[a] = degree.get(a, 0) + 1
    degree[b] = degree.get(b, 0) + 1

def build_graph(path):
    used = {}
    degree = {}

    for y in range(10):
        add_edge(used, degree, 0, y, 0, y + 1)
        add_edge(used, degree, 8, y, 8, y + 1)
    for x in range(3):
        add_edge(used, degree, x, 0, x + 1, 0)
        add_edge(used, degree, x, 10, x + 1, 10)
    for x in range(5, 8):
        add_edge(used, degree, x, 0, x + 1, 0)
        add_edge(used, degree, x, 10, x + 1, 10)
    for y in [-1, 10]:
        add_edge(used, degree, 3, y, 3, y + 1)
        add_edge(used, degree, 5, y, 5, y + 1)
    for x in range(3, 5):
        add_edge(used, degree, x, -1, x + 1, -1)
        add_edge(used, degree, x, 11, x + 1, 11)

    for edge in path:
        a = edge[0]
        b = edge[1]
        add_edge(used, degree, a["x"], a["y"], b["x"], b["y"])
    return (used, degree)

def valid_node(x, y):
    if x >= 0 and x <= 8 and y >= 0 and y <= 10:
        return True
    return x >= 3 and x <= 5 and (y == -1 or y == 11)

def valid_segment(ax, ay, bx, by):
    if not valid_node(ax, ay) or not valid_node(bx, by):
        return False
    mx = ax + bx
    my = ay + by
    in_pitch = mx >= 0 and mx <= 16 and my >= 0 and my <= 20
    in_north_goal = mx >= 6 and mx <= 10 and my >= -2 and my <= 0
    in_south_goal = mx >= 6 and mx <= 10 and my >= 20 and my <= 22
    return in_pitch or in_north_goal or in_south_goal

def legal_moves(x, y, used, degree):
    moves = []
    for direction in range(8):
        delta = DIRECTIONS[direction]
        nx = x + delta[0]
        ny = y + delta[1]
        if valid_segment(x, y, nx, ny) and edge_key(x, y, nx, ny) not in used:
            moves.append((direction, nx, ny, degree.get(point_key(nx, ny), 0) > 0, ny == -1 or ny == 11))
    return moves

def goal_is_ours(y, north):
    return (north and y == -1) or (not north and y == 11)

def advance(move, x, y, used, degree, our_turn, north, ply):
    nx = move[1]
    ny = move[2]
    if move[4]:
        value = WIN - ply if goal_is_ours(ny, north) else -WIN + ply
        return (value, None, None, nx, ny, our_turn, [])

    next_used = dict(used)
    next_degree = dict(degree)
    add_edge(next_used, next_degree, x, y, nx, ny)
    next_our_turn = our_turn if move[3] else not our_turn
    moves = legal_moves(nx, ny, next_used, next_degree)
    if len(moves) == 0:
        # The player who would act next is trapped, so the other player scores.
        value = -WIN + ply if next_our_turn else WIN - ply
        return (value, None, None, nx, ny, next_our_turn, moves)
    return (None, next_used, next_degree, nx, ny, next_our_turn, moves)

def evaluate(x, y, used, degree, our_turn, north):
    progress = 5 - y if north else y - 5
    center_distance = x - 4
    if center_distance < 0:
        center_distance = -center_distance

    moves = legal_moves(x, y, used, degree)
    bounce_count = 0
    for move in moves:
        if move[3]:
            bounce_count += 1

    score = progress * 140 - center_distance * 16
    # Initiative is useful only when there is room to act; forced bounce lanes
    # are handled by the search itself.
    if our_turn:
        score += len(moves) * 5 + bounce_count * 9
    else:
        score -= len(moves) * 5 + bounce_count * 9
    return score

def search_1(x, y, used, degree, our_turn, north, ply):
    moves = legal_moves(x, y, used, degree)
    best = -INF if our_turn else INF
    for move in moves:
        result = advance(move, x, y, used, degree, our_turn, north, ply)
        score = result[0]
        if score == None:
            score = evaluate(result[3], result[4], result[1], result[2], result[5], north)
        if our_turn and score > best:
            best = score
        if not our_turn and score < best:
            best = score
    return best

def choose_move(state):
    north = state["attacks"] == "north"
    x = state["ball"]["x"]
    y = state["ball"]["y"]
    used, degree = build_graph(state["path"])

    best_direction = state["legal_moves"][0]["direction"]
    best_score = -INF
    for move in legal_moves(x, y, used, degree):
        result = advance(move, x, y, used, degree, True, north, 1)
        score = result[0]
        if score == None:
            score = search_1(result[3], result[4], result[1], result[2], result[5], north, 2)
        if score > best_score:
            best_score = score
            best_direction = move[0]
    return best_direction
