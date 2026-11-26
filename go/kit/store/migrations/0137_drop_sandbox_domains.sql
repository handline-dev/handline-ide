-- sandbox_domains goes: a host an agent may reach from its sandbox is a host entry of its sandbox_allow list.
ALTER TABLE agents DROP COLUMN sandbox_domains;
