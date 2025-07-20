
class Node:
    def __init__(self, L, R, total):
        self.arr = []
        self.sum = total

    

class SegmentTree:
    def __init__(self, L, R, s):
        self.arr = []

    @staticmethod
    def builder(nums,i,l,r):

        if l==r:
            self.arr[i] = nums[l]
            return

        mid = (l+r)//2

        self.builder(nums,2*i+1, l, mid)
        self.builder(nums, 2*i+2, mid+1, r)
        self.arr[i] = self.arr[2*i+1] + self.arr[2*i+2]


        

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