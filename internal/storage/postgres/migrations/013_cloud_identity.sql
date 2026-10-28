-- Provisioning identity never shares a token, enrollment, revision or process lease
-- with the workload registrar, even for the same physical cluster.
CREATE TABLE access_cloud_clusters (LIKE access_clusters INCLUDING ALL);
CREATE TABLE access_cloud_enrollments (LIKE access_enrollments INCLUDING ALL);
ALTER TABLE access_cloud_enrollments ADD FOREIGN KEY (organization_id,cluster_id)
  REFERENCES access_cloud_clusters(organization_id,id);
CREATE TABLE access_cloud_agents (LIKE access_agents INCLUDING ALL);
ALTER TABLE access_cloud_agents ADD FOREIGN KEY (organization_id,cluster_id)
  REFERENCES access_cloud_clusters(organization_id,id);
