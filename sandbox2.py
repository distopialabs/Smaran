from sandbox import endingBlock, startingBlock


def getBalance(idx):
    return f"{idx}"


def getParent(idx):
    if idx & 1 == 0:
        return (idx - 2) // 2
    else:
        return (idx - 1) // 2


def update(idx, balance, tree):
    tree[idx] = balance
    while idx > 0:
        parentIdx = getParent(idx)

        leftChild = tree[2 * parentIdx + 1]
        rightChild = tree[2 * parentIdx + 2]

        if not leftChild or not rightChild:
            break

        tree[parentIdx] = [
            f"{leftChild[0]} + {rightChild[0]}",
            leftChild[1],
            rightChild[2],
        ]
        idx = parentIdx


segmentTree = [None] * (2 * 2048)
segStart = 0
segEnd = 199

blockStart = 40
blockEnd = 199

s = blockStart - segStart
e = blockEnd - segStart
# for blockNumber in range(s, e + 1):
#     idx = 2047 + blockNumber
#     val = [f"{idx}", idx, idx]
#     update(idx, val, segmentTree)

# print(segmentTree)


def collect_nodes(N, L, R):
    base = N - 1
    l = L + base
    r = R + base
    out = []

    while l <= r:
        print(l, r)
        if l % 2 == 0:
            out.append(l)
            l += 1
        if r % 2 == 1:
            out.append(r)
            r -= 1

        print("new", l, r)

        l = (l - 1) // 2
        r = (r - 1) // 2

    return out


nodes = collect_nodes(segEnd - segStart + 1, s, e)
print("Flat‐tree nodes:", nodes)

# for node in nodes:
#     print(segmentTree[node])
