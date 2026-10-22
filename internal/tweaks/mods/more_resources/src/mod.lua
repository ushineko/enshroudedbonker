-- @tweak name="More resources" group="Resources"
-- @desc More from everything you gather: each pickup a tree, rock or node drops,
-- @desc each plant or bush you harvest, and each voxel of terrain you dig. Also lets
-- @desc the world's "Resource drop amount" setting go up to 10x.
-- @param PICKUP_MULTIPLIER label="Pickups (stone, wood, resin)" min=1 max=20 step=1 default=3
-- @param HARVEST_MULTIPLIER label="Harvested plants and bushes" min=1 max=20 step=1 default=3
-- @param TERRAIN_FACTOR label="Terrain dug" min=1 max=20 step=1 default=3
-- @param DROP_MAX label="Highest resource-drop setting" min=2 max=10 step=0.5 default=10.0 kind=float
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local PICKUP_MULTIPLIER = 3
local HARVEST_MULTIPLIER = 3
local TERRAIN_FACTOR = 3
local DROP_MAX = 10.0

-- 1. Let the world setting go above 2x.
local presets = game.assets.get_resources_by_type("keen::GameSettingsPresetsResource")[1].data
presets.maxValues.resourceDropStackAmountFactor = DROP_MAX

-- The settings slider has its own fixed steps (2.0 down to 0.25); add the higher ones.
local DROP_STEPS = { 10.0, 7.5, 5.0, 4.0, 3.0, 2.5 }
local steps_added = 0
for _, r in ipairs(game.assets.get_resources_by_type("keen::FbUiBundle")) do
  local ok, ds = pcall(function() return r.data.difficultySettings end)
  if ok and ds ~= nil then
    local steps = ds.settingValues.dropAmount.steps   -- descending
    for i = #DROP_STEPS, 1, -1 do
      local v = DROP_STEPS[i]
      if v <= DROP_MAX and v > steps[1] + 1e-6 then
        table.insert(steps, 1, v); steps_added = steps_added + 1
      end
    end
  end
end

local function matches(name, patterns)
  for _, p in ipairs(patterns) do
    if string.match(name, p) then return true end
  end
  return false
end

local PICKUPS = { "^LootPickup_Gravity_", "^_LootPickup_Gravity_", "^LootPickup_Material_", "^Scatter_Loot_" }
local HARVEST = { "^Scatter_Loot_", "^LootPickup_" }
local NOT_HARVEST = { "^LootPickup_Collections", "^LootPickup_NPC", "^LootPickup_Equipment",
                      "^LootPickup_Ammo", "^LootPickup_Consumable", "^LootPickup_colorPalette" }

-- 2. Pickups give N items each (PickupItem.pickupAmount, u16).
-- 3. Remember which default inventories harvestables hand out.
local pickups, harvest_inventories = 0, {}
for _, r in ipairs(game.assets.get_resources_by_type("keen::ecs::TemplateResource")) do
  local t = r.data
  local is_pickup = matches(t.name, PICKUPS)
  local is_harvest = matches(t.name, HARVEST) and not matches(t.name, NOT_HARVEST)
  for _, comp in ipairs(t.components) do
    if is_pickup and comp.type == "keen::ecs::PickupItem" and comp.value.pickupAmount > 0 then
      comp.value.pickupAmount = math.min(comp.value.pickupAmount * PICKUP_MULTIPLIER, 65535)
      pickups = pickups + 1
    elseif is_harvest and comp.type == "keen::ecs::DefaultInventory" then
      harvest_inventories[tostring(comp.value.defaultInventory)] = true
    end
  end
end

local stacks = 0
local function scale_group(g)
  for _, s in ipairs(g.stacks) do
    s.countMin = s.countMin * HARVEST_MULTIPLIER
    s.countMax = s.countMax * HARVEST_MULTIPLIER
    stacks = stacks + 1
  end
  for _, child in ipairs(g.groups) do scale_group(child) end
end
local inventories = 0
for _, r in ipairs(game.assets.get_resources_by_type("keen::ecs::DefaultInventoryResource")) do
  if harvest_inventories[tostring(r.guid)] then
    scale_group(r.data.rootGroup)
    inventories = inventories + 1
  end
end

-- 4. Terrain: fewer voxels per item. Player-built blocks are left alone, so
--    tearing down a build never returns more than went into it.
local terra = game.assets.get_resources_by_type("keen::TerraformingEfficiencyRegistryResource")[1].data
local terrain = 0
for _, c in ipairs(terra.terrainConfigs) do
  c.terrainPerLootItemExchangeRate = math.max(1, math.floor(c.terrainPerLootItemExchangeRate / TERRAIN_FACTOR))
  terrain = terrain + 1
end

print(string.format("more_resources: drop max %.1f (+%d slider steps); pickups x%d on %d; harvest x%d on %d inventories (%d stacks); terrain /%d on %d",
  DROP_MAX, steps_added, PICKUP_MULTIPLIER, pickups, HARVEST_MULTIPLIER, inventories, stacks, TERRAIN_FACTOR, terrain))
