
class Node:
    def __init__(self, L, R, total):
        self.L = L
        self.R = R
        self.sum = sum
        self.leftChild = None
        self.rightChild = None

    

class SegmentTree:
    def __init__(self, L, R, s):
        self.L = L
        self.R = R
        self.sum=s
        self.leftChild = None
        self.rightChild = None

    @staticmethod
    def builder(nums, L, R):
        if L == R:
            return SegmentTree(L, R, nums[L])

        mid = (L + R) // 2
        left = SegmentTree.builder(nums, L, mid)
        right = SegmentTree.builder(nums, mid + 1, R)

        node = SegmentTree(L, R, left.sum + right.sum)
        node.leftChild = left
        node.rightChild = right

        return node

    def insert(self, index, val):
        pass    


    def update(self, index, val):

        if self.L == self.R and self.L == index:
            self.sum= val
            return
        
        mid = (root.L + root.R)//2

        if index<= mid:
            self.left.update(index, val)
        else:
            self.right.update(index, val)

        self.sum = self.left.sum+ self.right.sum

    def rangeQuery(self, L, R):

        """
        lL,Rr
        L,l m    m+1, R r
        xl, m    m+1, xR 
        L,x m    m+1, xR 

        """
        mid = (root.L + root.R)//2

        if L == self.L and self.R == R:
            return self.sum

        if R<=mid:
            return self.left.rangeQuery(L,R)
        
        if L >mid:
            return self.right.rangeQuery(L,R)

        return self.left.rangeQuery(L,mid) + self.right.rangeQuery(mid+1,R)
            






sgtree = SegmentTree.builder([1, 3, 5, 7, 9])