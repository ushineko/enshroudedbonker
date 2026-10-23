-- @tweak name="Fast-travel destinations" group="World"
-- @desc Makes the points of interest on the map fast-travel destinations: dungeons,
-- @desc locations, flame sites (shrines, obelisks, gem forges) and named areas
-- @desc (towns, districts, area names). It does not lift the in-Shroud, falling or
-- @desc underwater blocks. Travelling to a marker can land you outside the playable
-- @desc map or leave a gravestone unreachable.
-- @param DUNGEONS label="Dungeons" min=0 max=1 step=1 default=1 kind=bool
-- @param LOCATIONS label="Locations" min=0 max=1 step=1 default=1 kind=bool
-- @param FLAME_SITES label="Flame sites" min=0 max=1 step=1 default=1 kind=bool
-- @param AREAS label="Named areas" min=0 max=1 step=1 default=1 kind=bool
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local DUNGEONS = true
local LOCATIONS = true
local FLAME_SITES = true
local AREAS = true

-- Marker types that only objects the player builds use (water dispensers sit
-- in the flame category): not places to travel to.
local used, built = {}, {}
for _, r in ipairs(game.assets.get_resources_by_type("keen::ecs::TemplateResource")) do
  for _, comp in ipairs(r.data.components) do
    if comp.type == "keen::ecs::MapMarker" then
      local id = game.guid.hash(comp.value.markerType)
      if r.data.name:sub(-7) == "_PLAYER" then
        built[id] = true
      else
        used[id] = true
      end
    end
  end
end

-- Uncategorised types are a mix. A text label without a range is a named
-- place; a range marks a search area, and the rest are props, the player's
-- gravestone and test markers.
local function wanted(m)
  local cat = m.sortingCategory
  if cat == "Dungeons" then return DUNGEONS end
  if cat == "Locations" then return LOCATIONS end
  if cat == "FlameRelated" then
    local id = m.markerId.value
    return FLAME_SITES and not (built[id] and not used[id])
  end
  if cat == "None" then
    return AREAS and m.showLabel and not m.showRange
  end
  return false
end

local reg = game.assets.get_resources_by_type("keen::MapMarkerRegistryResource")[1].data
local n = 0
for _, m in ipairs(reg.mapMarkers.values) do
  if not m.isCustomMarkerType and not m.isFastTravelDestination and wanted(m) then
    m.isFastTravelDestination = true
    n = n + 1
  end
end
print("fast_travel_markers: enabled " .. n .. " marker types")
