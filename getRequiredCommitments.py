startingBlock = 6100
endingBlock = 22_000_000

l1Commitments = []
l2Commitments = []
l3Commitments = []
l4Commitments = []

"""

1️⃣ (IN SAME C and has any Frag)
left frag + right frag + sb and eb in same C' ✅
    - 1 L1 commitment for sb - eb
    - **no need upper layer 
    - NEED upper layer to interpolate lower layer commit ONLY

left frag only + sb and eb in same C' (IN SAME C and has any Frag)✅
    - 1 L1 commitment for sb - leftFragEnd (eb)
    - **no need upper layer 
    - NEED upper layer to interpolate lower layer commit ONLY

right frag only + sb and eb in same C': (IN SAME C and has any Frag)✅
    - 1 L1 commitment for rightFragStart (sb) - eb
    - **no need upper layer 
    - NEED upper layer to interpolate lower layer commit ONLY



2️⃣ 
left frag only + sb and eb in different C'
    - 1 L1 commitment for sb - leftFragEnd
    - need upper layer if block remaining in between leftFragEnd+1 - eb 
    - NEED upper layer to interpolate lower layer commit as well

right frag only + sb and eb in different C':
    - 1 L1 commitment for rightFragStart - eb
    - need upper layer if block remaining in between sb - rightFragStart-1
    - NEED upper layer to interpolate lower layer commit as well


left frag + right frag + sb and eb in different C':
    - 1 L1 commitment for sb - leftFragEnd
    - 1 L1 commitment for rightFragStart - eb
    - need upper layer if block remaining in between leftFragEnd+1 - rightFragStart-1 (need to inteprolate lower layer commit)
    - NEED upper layer to interpolate lower layer commit as well




3️⃣ NO FRAG
no frag + sb and eb in same C': ✅
    - No L1 commitment
    - need upper layer 
    - NO NEED to interpolate lower layer commit

no frag + sb and eb in different C': ✅
    - No L1 commitment
    - need upper layer 
    - NO NEED to interpolate lower layer commit










"""


"""

RangeC

{
    1:[
        {
            type: range
            idx: 
        }
    ]
    2:[]
    3:
    4:
}


6100-6143

segmenttree: 4096 - 6143

generate segment tree

find the nodes required for generating witness
generate witness
"""


def getCommitments(startingBlock, endingBlock, layer=1):
    sb = startingBlock
    eb = endingBlock

    # For L1

    hasL1LeftFragment = sb % 2048 != 0
    hasL1RightFragment = eb % 2048 != 2047

    if not hasL1LeftFragment and not hasL1RightFragment:
        # No l1 commitment, move to upper layer
        # getLxCommitments(sb,eb,2)
        print(f"Need upper layer commitments for {sb} - {eb}")
        # TODO: need upper layer commitments for sb-eb
        return

    leftCommitmentIndex = sb // 2048
    rightCommitIndex = eb // 2048

    #  sb and eb in same C', no need upper layer commitments
    if leftCommitmentIndex == rightCommitIndex:
        l1Commitments.append(f"L1 Commitment for {sb} - {eb}")
        # No need upper layer
        return

    # has either leftfragment or right fragment or both
    # sb and eb in different C'
    if hasL1LeftFragment:
        leftFragmentStart = sb
        leftFragmentEnd = (leftCommitmentIndex + 1) * 2048 - 1
        l1Commitments.append(
            f"L1 Commitment for {leftFragmentStart} - {leftFragmentEnd}"
        )
        sb = leftFragmentEnd + 1

    if hasL1RightFragment:
        rightFragmentStart = rightCommitIndex * 2048
        rightFragmentEnd = eb
        l1Commitments.append(
            f"L1 Commitment for {rightFragmentStart} - {rightFragmentEnd}"
        )
        eb = rightFragmentStart - 1

    if sb < eb:
        print(f"nneed upper layer commitments for {sb} - {eb}")

        # TODO: need upper layer commitments for sb-eb
        pass


def findSegmentTree(idx, layer):
    start = idx * 2048 * pow(1365, layer - 1)
    end = (idx + 1) * 2048 * pow(1365, layer - 1) - 1

    return f"{start} - {end}: size {end-start+1}"


def findRequiredCommitments(sb, eb, layer=1):

    l0BatchSize = 2048 * pow(1365, layer - 1)

    hasLeftFragment = sb % (l0BatchSize) != 0  # not first element of the batch
    hasRightFragment = eb % (l0BatchSize) != (l0BatchSize - 1)

    leftCommitmentIndex = sb // (l0BatchSize)
    rightCommitIndex = eb // (l0BatchSize)

    #  sb and eb in same C', no need upper layer commitments
    if leftCommitmentIndex == rightCommitIndex and (
        hasLeftFragment or hasRightFragment
    ):
        l1Commitments.append([leftCommitmentIndex, sb, eb, layer])
        # l1Commitments.append(
        #     f"Layer{layer} Commitment (idx: {leftCommitmentIndex}) for {sb} - {eb}, segment tree: {findSegmentTree(leftCommitmentIndex, layer)}"
        # )
        # No need upper layer
        return

    # has either leftfragment or right fragment or both
    # sb and eb in different C'
    if hasLeftFragment:
        leftFragmentStart = sb
        leftFragmentEnd = (leftCommitmentIndex + 1) * l0BatchSize - 1
        l1Commitments.append(
            [leftCommitmentIndex, leftFragmentStart, leftFragmentEnd, layer]
        )

        # l1Commitments.append(
        #     f"Layer{layer} Commitment (idx: {leftCommitmentIndex}) for {leftFragmentStart} - {leftFragmentEnd}, segment tree: {findSegmentTree(leftCommitmentIndex, layer)}"
        # )
        sb = leftFragmentEnd + 1

    if hasRightFragment:
        rightFragmentStart = rightCommitIndex * l0BatchSize
        rightFragmentEnd = eb
        l1Commitments.append(
            [rightCommitIndex, rightFragmentStart, rightFragmentEnd, layer]
        )
        # l1Commitments.append(
        #     f"Layer{layer} Commitment (idx: {rightCommitIndex}) for {rightFragmentStart} - {rightFragmentEnd}, segment tree: {findSegmentTree(rightCommitIndex, layer)}"
        # )
        eb = rightFragmentStart - 1

    if sb < eb:
        findRequiredCommitments(sb, eb, layer + 1)


findRequiredCommitments(startingBlock, endingBlock, 1)
# getCommitments(startingBlock, endingBlock, 1)

for idx, sb, eb, layer in l1Commitments:
    print(f"Layer{layer} Commitment (idx: {idx}) for {sb} - {eb}")
    print(f"Segment Tree: {findSegmentTree(idx, layer)} \n")
    findSegmentTree(idx, layer)
