-- Adds the customer's contact and the cash the courier collects on delivery.
-- New databases get these columns from init.sql; run this once on a database created before them:
--   docker exec -i tracker-mysql mysql -uroot -proot < db/migrations/001_customer_and_collect.sql
USE package_tracking;

ALTER TABLE orders
  ADD COLUMN customer_name  VARCHAR(100)  NOT NULL DEFAULT '' AFTER dropoff_location,
  ADD COLUMN customer_phone VARCHAR(20)   NOT NULL DEFAULT '' AFTER customer_name,
  ADD COLUMN collect_amount DECIMAL(10,2) NOT NULL DEFAULT 0  AFTER delivery_fee;
