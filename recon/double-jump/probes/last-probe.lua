local A = game.assets
for _, r in ipairs(A.get_resources_by_type("keen::actor::ActorSequenceResource")) do
  local n = tostring(r.data.subSequences[1] and r.data.subSequences[1].name)
  if n:find("Ability_Movement") and n:lower():find("jump") then print("ID seq " .. n .. " " .. tostring(game.guid.hash(tostring(r.guid)))) end
end
for _, r in ipairs(A.get_resources_by_type("keen::ItemInfo")) do
  local n = tostring(r.data.debugName)
  if n == "Ability_DoubleJump" or n == "Ability_SprintJump" or n == "Ability_MountainGoatingJump" then print("ID item " .. n .. " " .. tostring(r.data.itemId.value)) end
end
