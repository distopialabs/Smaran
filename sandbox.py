startingBlock = 1024
endingBlock = 6100

l1Commitments = []
l2Commitments = []
l3Commitments = []
l4Commitments = []

"""

left frag + right frag + sb and eb in same C'
    - 1 L1 commitment for sb - eb
    - no need upper layer


left frag + right frag + sb and eb in different C':
    - 1 L1 commitment for sb - leftFragEnd
    - 1 L1 commitment for rightFragStart - eb
    - need upper layer if block remaining in between leftFragEnd+1 - rightFragStart-1 


left frag only + sb and eb in same C'
    - 1 L1 commitment for sb - leftFragEnd
    - (NO NEED) need upper layer if block remaining in between leftFragEnd+1 - eb 

left frag only + sb and eb in different C'
    - 1 L1 commitment for sb - leftFragEnd
    - need upper layer if block remaining in between leftFragEnd+1 - eb 


right frag only + sb and eb in same C':
    - 1 L1 commitment for rightFragStart - eb
    - (NO NEED) need upper layer if block remaining in between sb - rightFragStart-1


right frag only + sb and eb in different C':
    - 1 L1 commitment for rightFragStart - eb
    - need upper layer if block remaining in between sb - rightFragStart-1







no frag + sb and eb in same C': ✅
    - No L1 commitment

no frag + sb and eb in different C': ✅
    - No L1 commitment


"""


def getCommitments(startingBlock, endingBlock):
    sb = startingBlock
    eb = endingBlock

    # For L1

    hasL1LeftFragment = sb % 2048 != 0
    hasL1RightFragment = eb % 2048 != 2047

    if not hasL1LeftFragment and not hasL1RightFragment:
        # No l1 commitment, move to upper layer
        pass

    leftCommitmentIndex = sb // 2048
    rightCommitIndex = eb // 2048

    if leftCommitmentIndex == rightCommitIndex:
        l1Commitments.append(f"Commitment for {sb} - {eb}")
        return

    if hasL1LeftFragment and hasL1RightFragment:
        leftCommitmentIndex = sb // 2048
        rightCommitIndex = eb // 2048

        if leftCommitmentIndex == rightCommitIndex:
            l1Commitments.append(f"Commitment for {sb} - {eb}")
            return
        else:
            leftFragmentStart = sb
            leftFragmentEnd = (leftCommitmentIndex + 1) * 2048 - 1
            rightFragmentStart = rightCommitIndex * 2048
            rightFragmentEnd = eb
            l1Commitments.append(
                f"Commitment for {leftFragmentStart} - {leftFragmentEnd}"
            )
            l1Commitments.append(
                f"Commitment for {rightFragmentStart} - {rightFragmentEnd}"
            )

    elif hasL1LeftFragment:
        leftCommitmentIndex = sb // 2048
        rightCommitIndex = eb // 2048

        if leftCommitmentIndex == rightCommitIndex:
            l1Commitments.append(f"Commitment for {sb} - {eb}")
            return
        else:
            leftFragmentStart = sb
            leftFragmentEnd = (leftCommitmentIndex + 1) * 2048 - 1
            rightFragmentStart = rightCommitIndex * 2048
            rightFragmentEnd = eb
            l1Commitments.append(
                f"Commitment for {leftFragmentStart} - {leftFragmentEnd}"
            )
            l1Commitments.append(
                f"Commitment for {rightFragmentStart} - {rightFragmentEnd}"
            )
    elif hasL1RightFragment:
        pass
    else:
        pass

    if hasL1LeftFragment:
        l1CommitmentIndex = sb // 2048
        l1LeftFragmentStart = sb
        l1LeftFragmentEnd = (l1CommitmentIndex + 1) * 2048 - 1

        if endingBlock <= l1LeftFragmentEnd:
            l1Commitments.append(
                f"Commitment for {l1LeftFragmentStart} - {endingBlock}"
            )
            return

        l1Commitments.append(
            f"Commitment for {l1LeftFragmentStart} - {l1LeftFragmentEnd}"
        )
        sb = l1LeftFragmentEnd + 1

    if hasL1RightFragment:
        l1CommitIndex = eb // 2048
        l1RightFragmentStart = l1CommitIndex * 2048
        l1RightFragmentEnd = eb

        if l1RightFragmentStart <= startingBlock:
            l1Commitments.append(
                f"Commitment from {startingBlock} - {l1RightFragmentEnd}"
            )
            return
        l1Commitments.append(
            f"Commitment from {l1RightFragmentStart} - {l1RightFragmentEnd}"
        )
        eb = l1RightFragmentStart - 1

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


getCommitments(startingBlock, endingBlock)


print(l1Commitments)
print(l2Commitments)
print(l3Commitments)
print(l4Commitments)
