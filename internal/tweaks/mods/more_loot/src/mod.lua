-- @tweak name="More loot" group="Loot"
-- @desc More loot rolls per enemy kill, and bigger stacks of every stackable
-- @desc material that drops as loot. The stack size applies to chests too.
-- @param ENEMY_SLOT_MULTIPLIER label="Rolls per kill" min=1 max=8 step=1 default=2
-- @param LOOT_STACK_MULTIPLIER label="Material stack size" min=1 max=20 step=1 default=2
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local ENEMY_SLOT_MULTIPLIER = 2
local LOOT_STACK_MULTIPLIER = 2

-- 1. More rolls per kill. Only enemies: chests and loot nodes keep theirs.
local enemies = 0
for _, r in ipairs(game.assets.get_resources_by_type("keen::ecs::TemplateResource")) do
  local t = r.data
  if string.match(t.name, "^Enemy_") then
    for _, comp in ipairs(t.components) do
      if comp.type == "keen::ecs::RandomLootSlotCount" then
        comp.value.slotsMin = math.min(comp.value.slotsMin * ENEMY_SLOT_MULTIPLIER, 16)
        comp.value.slotsMax = math.min(comp.value.slotsMax * ENEMY_SLOT_MULTIPLIER, 16)
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
