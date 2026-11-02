-- Throwaway experiment, not a built-in: edit the three values, reimport, rebuild.
local JUMP_DELAY_NS = 500000000 -- second jump's rise starts this long after the press
local AIR_SPEED = 5      -- movement factors during the second jump (vanilla 1)
local COST = 1           -- stamina per double jump (vanilla 10)
local FALL_FACTOR = 10   -- multiplies the fall distances where damage starts and turns lethal

local function find_sequence(name)
  for _, r in ipairs(game.assets.get_resources_by_type("keen::actor::ActorSequenceResource")) do
    for _, sub in ipairs(r.data.subSequences) do
      if sub.name == name then return sub end
    end
  end
  error("double_jump_experiment: no sequence " .. name)
end

local sub = find_sequence("Ability_Movement_Skill_JumpDoubleSecond")
-- Control test: edit only events the game already has, and add none.
local jump_i
for i, ev in ipairs(sub.events) do
  if ev.type == "keen::actor::JumpEvent" then jump_i = i end
  if ev.type == "keen::actor::LerpMovementFactorEvent" then
    local m = ev.value.desiredMovementFactors
    m.forward, m.backward, m.lateral = AIR_SPEED, AIR_SPEED, AIR_SPEED
  end
end
assert(jump_i, "double_jump_experiment: jump event missing")
-- Delay the jump, keeping the list in time order.
local jump = table.remove(sub.events, jump_i)
jump.value.time = { value = JUMP_DELAY_NS }
local at = 0
for i, ev in ipairs(sub.events) do
  if ev.value.time.value <= JUMP_DELAY_NS then at = i end
end
table.insert(sub.events, at + 1, jump)

local cost_done = false
for _, r in ipairs(game.assets.get_resources_by_type("keen::ItemInfo")) do
  if r.data.debugName == "Ability_DoubleJump" then
    r.data.impactValues.simple[1].value.value = COST
    cost_done = true
  end
end
assert(cost_done, "double_jump_experiment: no Ability_DoubleJump")

local start, lethal, n
n = 0
for _, r in ipairs(game.assets.get_resources_by_type("keen::ecs::TemplateResource")) do
  if r.data.name == "1_Player" or r.data.name == "1_Player_AG2" then
    for _, c in ipairs(r.data.components) do
      if c.type == "keen::ecs::FallDamage" then
        c.value.fallDamageStartDistance = c.value.fallDamageStartDistance * FALL_FACTOR
        c.value.fallDamageLethalDistance = c.value.fallDamageLethalDistance * FALL_FACTOR
        start, lethal = c.value.fallDamageStartDistance, c.value.fallDamageLethalDistance
        n = n + 1
      end
    end
  end
end
assert(n > 0, "double_jump_experiment: no player FallDamage")
print("double_jump_experiment: jump delay " .. JUMP_DELAY_NS .. "ns, air speed x" .. AIR_SPEED .. ", cost " .. COST .. ", fall damage " .. start .. ".." .. lethal .. " on " .. n .. " templates")
