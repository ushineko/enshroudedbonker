local RARE_MULTIPLIER = 1.5        -- weight of Rare in random loot (vanilla 17.1)
local EPIC_MULTIPLIER = 2.0        -- weight of Epic (vanilla 9.7)
local LEGENDARY_MULTIPLIER = 2.0   -- weight of Legendary (vanilla 5.3)
local CHANCE_MULTIPLIER = 2.0      -- chance of the less-than-certain entries in fixed item sets (0.08, 0.2, 0.5 ...), capped at 1
local GEM_SLOT_MULTIPLIER = 1.0    -- chance of a gem slot by rarity, capped at 1 (1.0 = vanilla)

-- 1. Rarity split for random loot: percentage weights Common..Legendary.
local db = game.assets.get_resources_by_type("keen::LootableItemsResource")[1].data
local w = db.globalRarityDropRates
local before = string.format("%.1f/%.1f/%.1f/%.1f/%.1f", w[1], w[2], w[3], w[4], w[5])
w[3] = w[3] * RARE_MULTIPLIER
w[4] = w[4] * EPIC_MULTIPLIER
w[5] = w[5] * LEGENDARY_MULTIPLIER
local after = string.format("%.1f/%.1f/%.1f/%.1f/%.1f", w[1], w[2], w[3], w[4], w[5])

-- 2. Fixed item sets (chests, harvestables, corpses): raise the chance of
--    entries that are not already certain.
local entries = 0
local function scale_group(g)
  for _, s in ipairs(g.stacks) do
    if s.spawnProbability > 0 and s.spawnProbability < 1 then
      s.spawnProbability = math.min(1.0, s.spawnProbability * CHANCE_MULTIPLIER)
      entries = entries + 1
    end
  end
  for _, child in ipairs(g.groups) do scale_group(child) end
end
if CHANCE_MULTIPLIER ~= 1.0 then
  for _, r in ipairs(game.assets.get_resources_by_type("keen::ecs::DefaultInventoryResource")) do
    scale_group(r.data.rootGroup)
  end
end

-- 3. Gem slots.
if GEM_SLOT_MULTIPLIER ~= 1.0 then
  local g = game.assets.get_resources_by_type("keen::BalancingTable")[1].data.gemSlotProbability
  for _, k in ipairs({ "common", "uncommon", "rare", "epic", "legendary" }) do
    g[k] = math.min(1.0, g[k] * GEM_SLOT_MULTIPLIER)
  end
end

print("better_loot: rarity weights " .. before .. " -> " .. after .. "; chances x" .. CHANCE_MULTIPLIER .. " on " .. entries .. " entries; gem slots x" .. GEM_SLOT_MULTIPLIER)
