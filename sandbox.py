startingBlock = 6_100
endingBlock = 22_000_000

l1Commitments = []
l2Commitments = []
l3Commitments = []
l4Commitments = []

"""
no frag + sb and eb in same C': ✅
    - No L1 commitment
    - need upper layer

no frag + sb and eb in different C': ✅
    - No L1 commitment
    - need upper layer


left frag + right frag + sb and eb in same C'✅
    - 1 L1 commitment for sb - eb
    - no need upper layer

left frag only + sb and eb in same C'✅
    - 1 L1 commitment for sb - leftFragEnd (eb)
    - (NO NEED) need upper layer if block remaining in between leftFragEnd+1 - eb 

right frag only + sb and eb in same C':✅
    - 1 L1 commitment for rightFragStart (sb) - eb
    - (NO NEED) need upper layer if block remaining in between sb - rightFragStart-1




left frag + right frag + sb and eb in different C':
    - 1 L1 commitment for sb - leftFragEnd
    - 1 L1 commitment for rightFragStart - eb
    - need upper layer if block remaining in between leftFragEnd+1 - rightFragStart-1 



left frag only + sb and eb in different C'
    - 1 L1 commitment for sb - leftFragEnd
    - need upper layer if block remaining in between leftFragEnd+1 - eb 




right frag only + sb and eb in different C':
    - 1 L1 commitment for rightFragStart - eb
    - need upper layer if block remaining in between sb - rightFragStart-1






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


def getLxCommitments(sb, eb, layer=1):
    # For L1

    hasLeftFragment = sb % (2048 * pow(1365, layer - 1)) != 0
    hasRightFragment = eb % (2048 * pow(1365, layer - 1)) != (
        2048 * pow(1365, layer - 1) - 1
    )

    if not hasLeftFragment and not hasRightFragment:
        # No l1 commitment, move to upper layer
        # getLxCommitments(sb,eb,2)
        # TODO: need upper layer commitments for sb-eb
        getLxCommitments(sb, eb, layer + 1)
        return

    leftCommitmentIndex = sb // (2048 * pow(1365, layer - 1))
    rightCommitIndex = eb // (2048 * pow(1365, layer - 1))

    #  sb and eb in same C', no need upper layer commitments
    if leftCommitmentIndex == rightCommitIndex and (
        hasLeftFragment or hasRightFragment
    ):
        l1Commitments.append(f"Layer{layer} Commitment for {sb} - {eb}")
        # No need upper layer
        return

    # has either leftfragment or right fragment or both
    # sb and eb in different C'
    if hasLeftFragment:
        leftFragmentStart = sb
        leftFragmentEnd = (leftCommitmentIndex + 1) * 2048 * pow(1365, layer - 1) - 1
        l1Commitments.append(
            f"Layer{layer} Commitment for {leftFragmentStart} - {leftFragmentEnd}"
        )
        sb = leftFragmentEnd + 1

    if hasRightFragment:
        rightFragmentStart = rightCommitIndex * 2048 * pow(1365, layer - 1)
        rightFragmentEnd = eb
        l1Commitments.append(
            f"Layer{layer} Commitment for {rightFragmentStart} - {rightFragmentEnd}"
        )
        eb = rightFragmentStart - 1

    if sb < eb:
        # TODO: need upper layer commitments for sb-eb
        getLxCommitments(sb, eb, layer + 1)


def getL2Commitments(sb, eb, layer=2):
    # For L2

    hasL2Fragment = sb % (2048 * 1365) != 0
    if hasL2Fragment:

        l2CommitmentIndex = sb // (2048 * 1365)
        l2FragmentStart = sb
        l2FragmentEnd = (l2CommitmentIndex + 1) * (2048 * 1365) - 1

        if l2FragmentEnd >= endingBlock:
            l2Commitments.append(f"Commitment for {l2FragmentStart} - {endingBlock}")
            return

        l2Commitments.append(f"Commitment for {l2FragmentStart} - {l2FragmentEnd}")
        sb = l2FragmentEnd + 1

    # For L3

    hasL3Fragment = sb % (2048 * 1365 * 1365) != 0
    if hasL3Fragment:
        l3CommitmentIndex = sb // (2048 * 1365 * 1365)
        l3FragmentStart = sb
        l3FragmentEnd = (l3CommitmentIndex + 1) * (2048 * 1365 * 1365) - 1

        if l3FragmentEnd >= endingBlock:
            l3Commitments.append(f"Commitment for {l3FragmentStart} - {endingBlock}")
            return

        l3Commitments.append(f"Commitment for {l3FragmentStart} - {l3FragmentEnd}")
        sb = l3FragmentEnd + 1

    # for L4

    hasL4Fragment = sb % (2048 * 1365 * 1365 * 1365) != 0
    if hasL4Fragment:
        l4CommitmentIndex = sb // (2048 * 1365 * 1365 * 1365)
        l4FragmentStart = sb
        l4FragmentEnd = (l4CommitmentIndex + 1) * (2048 * 1365 * 1365 * 1365) - 1

        if l4FragmentEnd >= endingBlock:
            l4Commitments.append(f"Commitment for {l4FragmentStart} - {endingBlock}")
            return

        l4Commitments.append(f"Commitment for {l4FragmentStart} - {l4FragmentEnd}")
        sb = l4FragmentEnd + 1


getLxCommitments(startingBlock, endingBlock, 1)
# getCommitments(startingBlock, endingBlock, 1)


print(l1Commitments)
print(l2Commitments)
print(l3Commitments)
print(l4Commitments)
