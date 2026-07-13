

## Logic for finding the commitments required to prove a range.


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










