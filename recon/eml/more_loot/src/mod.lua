local ENEMY_SLOT_MULTIPLIER = 2   -- loot rolls per enemy kill (Enemy_* RandomLootSlotCount)
local ENEMY_SLOT_CAP = 16         -- a loot container holds at most 16 slots
local LOOT_STACK_MULTIPLIER = 2   -- size of each stackable drop (materials); enemies AND chests

-- 1. More rolls per kill. Only enemies: chests and loot nodes keep theirs.
local enemies = 0
for _, r in ipairs(game.assets.get_resources_by_type("keen::ecs::TemplateResource")) do
  local t = r.data
  if string.match(t.name, "^Enemy_") then
    for _, comp in ipairs(t.components) do
      if comp.type == "keen::ecs::RandomLootSlotCount" then
        comp.value.slotsMin = math.min(comp.value.slotsMin * ENEMY_SLOT_MULTIPLIER, ENEMY_SLOT_CAP)
        comp.value.slotsMax = math.min(comp.value.slotsMax * ENEMY_SLOT_MULTIPLIER, ENEMY_SLOT_CAP)
        enemies = enemies + 1
      end
    end
  end
end

-- 2. Bigger stacks of stackable loot (gear, which doesn't stack, is untouched).
--    stackSizeMin/Max are u16; the scaled cap keeps its relation to the max.
-- Most materials drop as 1-1 in this table (spores, cloth, bones...), so
-- select by whether the item itself stacks, not by its loot range.
local stackable = {}
for _, r in ipairs(game.assets.get_resources_by_type("keen::ItemInfo")) do
  if r.data.maxStackSize > 1 then stackable[r.data.itemId.value] = true end
end
local db = game.assets.get_resources_by_type("keen::LootableItemsResource")[1].data
local stacks = 0
for _, it in ipairs(db.items) do
  if stackable[it.itemId.value] then
    it.stackSizeMin = math.min(it.stackSizeMin * LOOT_STACK_MULTIPLIER, 65535)
    it.stackSizeMax = math.min(it.stackSizeMax * LOOT_STACK_MULTIPLIER, 65535)
    it.stackSizeMaxScaled = math.min(it.stackSizeMaxScaled * LOOT_STACK_MULTIPLIER, 65535)
    stacks = stacks + 1
  end
end
print(string.format("more_loot: enemy rolls x%d on %d; loot stacks x%d on %d items",
  ENEMY_SLOT_MULTIPLIER, enemies, LOOT_STACK_MULTIPLIER, stacks))
