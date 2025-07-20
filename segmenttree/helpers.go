package segmenttree

func GetAncestors(nodeIdx int) []int {
	ancestors := []int{}
	for nodeIdx > 0 {
		parentNodeIdx := GetParent(nodeIdx)
		ancestors = append(ancestors, parentNodeIdx)
		nodeIdx = parentNodeIdx
	}
	return ancestors
}

func GetParent(nodeIdx int) int {
	if nodeIdx&1 == 0 { // if even, node is right child
		return (nodeIdx - 2) / 2
	} else { // if odd, node is left child
		return (nodeIdx - 1) / 2
	}
}

func getLeftChild(nodeIdx int) int {
	return 2*nodeIdx + 1
}

func getRightChild(nodeIdx int) int {
	return 2*nodeIdx + 2
}

func getSibling(nodeIdx int) int {
	if nodeIdx == 0 {
		return 0
	}
	if nodeIdx&1 == 0 {
		return nodeIdx - 1
	} else {
		return nodeIdx + 1
	}
}
