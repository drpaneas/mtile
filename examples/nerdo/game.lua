function _init()
  counter=0
  idle={
    fr1={0,8,40,80},
    fr2={40,8,40,80},
    fr3={80,8,40,80},
    fr4={120,8,40,80}
    }

end

function _update()
  counter += 1
  if counter == 5*4+1 then
    counter=1
  end
  
end

function _draw()
  cls(0)
  if counter <=5 then sspr(idle.fr1[1],idle.fr1[2],idle.fr1[3],idle.fr1[4], 10, 10)
  elseif counter <= 10 then  sspr(idle.fr2[1],idle.fr2[2],idle.fr2[3],idle.fr2[4], 10, 10) 
  elseif counter <= 15 then sspr(idle.fr3[1],idle.fr3[2],idle.fr3[3],idle.fr3[4], 10, 10)
  else sspr(idle.fr4[1],idle.fr4[2],idle.fr4[3],idle.fr4[4], 10, 10) end
  
  
end
