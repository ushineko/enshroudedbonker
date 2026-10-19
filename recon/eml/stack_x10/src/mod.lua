local n = 0
for _, item in ipairs(game.assets.get_resources_by_type("keen::ItemInfo")) do
  local d = item.data
  if d.maxStackSize > 1 then d.maxStackSize = math.min(d.maxStackSize * 10, 65535); n = n + 1 end
end
print("stack_x10: patched " .. n)
